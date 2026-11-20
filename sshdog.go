// Copyright 2016 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// TODO: High-level file comment.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Matir/sshdog/daemon"
)

type Debugger bool

func (d Debugger) Debug(format string, args ...interface{}) {
	if d {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", msg)
	}
}

var dbg Debugger = true

// configBox reads configuration from a plain directory on disk.
//
// This replaces go.rice: rice's init() inspects the running executable
// as an appended zip (via go.zipexe/debug-elf) and crashes with a nil
// pointer dereference on Android before main() even runs.  Reading a
// real directory needs no such tricks and works everywhere.
type configBox struct{ dir string }

func (b *configBox) Bytes(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(b.dir, name))
}

func (b *configBox) String(name string) (string, error) {
	data, err := b.Bytes(name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Lookup the port number
func getPort(box *configBox) int16 {
	if len(os.Args) > 1 {
		if port, err := strconv.Atoi(os.Args[1]); err != nil {
			dbg.Debug("Error parsing %s as port: %v", os.Args[1], err)
		} else {
			return int16(port)
		}
	}
	if portData, err := box.String("port"); err == nil {
		portData = strings.TrimSpace(portData)
		if port, err := strconv.Atoi(portData); err != nil {
			dbg.Debug("Error parsing %s as port: %v", portData, err)
		} else {
			return int16(port)
		}
	}
	return 2222 // default
}

// Just check if a file exists
func fileExists(box *configBox, name string) bool {
	_, err := box.Bytes(name)
	return err == nil
}

// Should we daemonize?
func shouldDaemonize(box *configBox) bool {
	return fileExists(box, "daemon")
}

// Should we be silent?
func beQuiet(box *configBox) bool {
	return fileExists(box, "quiet")
}

var mainBox *configBox

func main() {
	mainBox = mustFindBox()

	if beQuiet(mainBox) {
		dbg = false
	}

	if shouldDaemonize(mainBox) {
		if err := daemon.Daemonize(daemonStart); err != nil {
			dbg.Debug("Error daemonizing: %v", err)
		}
	} else {
		waitFunc, _ := daemonStart()
		if waitFunc != nil {
			waitFunc()
		}
	}
}

func mustFindBox() *configBox {
	// Search order: $SSHDOG_CONFIG_DIR, ./config next to the
	// executable, then ./config in the working directory.
	candidates := []string{}
	if dir := os.Getenv("SSHDOG_CONFIG_DIR"); dir != "" {
		candidates = append(candidates, dir)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "config"))
	}
	candidates = append(candidates, "config")
	for _, dir := range candidates {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			dbg.Debug("Using config directory: %s", dir)
			return &configBox{dir: dir}
		}
	}
	dbg.Debug("No config directory found (looked in: %v); "+
		"only defaults will apply.", candidates)
	return &configBox{dir: candidates[len(candidates)-1]}
}

// Actually run the implementation of the daemon
func daemonStart() (waitFunc func(), stopFunc func()) {
	server := NewServer()

	hasHostKeys := false
	for _, keyName := range keyNames {
		if keyData, err := mainBox.Bytes(keyName); err == nil {
			dbg.Debug("Adding hostkey file: %s", keyName)
			if err = server.AddHostkey(keyData); err != nil {
				dbg.Debug("Error adding public key: %v", err)
			}
			hasHostKeys = true
		}
	}
	if !hasHostKeys {
		if err := server.RandomHostkey(); err != nil {
			dbg.Debug("Error adding random hostkey: %v", err)
			return
		}
	}

	// Password auth, as in the sshdog_ios fork: when a file named
	// "password" exists in the config directory, its contents are the
	// login password.  May be combined with authorized_keys.
	authSet := false
	if passwordData, err := mainBox.Bytes("password"); err == nil {
		dbg.Debug("Setting auth password.")
		server.SetAuthPassword(passwordData)
		authSet = true
	}
	if authData, err := mainBox.Bytes("authorized_keys"); err == nil {
		dbg.Debug("Adding authorized_keys.")
		server.AddAuthorizedKeys(authData)
		authSet = true
	}
	if !authSet {
		// Unlike the iOS fork (which serves with no auth in this
		// case), refuse: an unauthenticated SSH server is never the
		// right default.
		dbg.Debug("Neither password nor authorized_keys was " +
			"configured. Refusing to serve without authentication.")
		return
	}
	server.ListenAndServe(getPort(mainBox))
	return server.Wait, server.Stop
}
