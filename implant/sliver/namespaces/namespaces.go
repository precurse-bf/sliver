package namespaces

/*
	Sliver Implant Framework
	Copyright (C) 2024  Bishop Fox

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	// {{if .Config.Debug}}

	"log"

	// {{end}}

	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// NamespaceType represents different types of namespaces
type NamespaceType int

const (
	NetworkNamespace NamespaceType = iota
	MountNamespace
)

const procDir = "/proc"

func GetFdFromPath(path string) (int, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	return fd, nil
}

func GetUniqueFd(fd int) string {
	// Returns the unique namespace ID
	var s unix.Stat_t

	err := unix.Fstat(fd, &s)

	if err != nil {
		return "Unknown"
	}

	return fmt.Sprintf("NS(%d:%d)", s.Dev, s.Ino)
}

func EnterNs(nsFd int, namespaceType NamespaceType) error {
	err := unix.Setns(nsFd, unix.CLONE_NEWNET)

	if err != nil {
		// Failed to enter namespace
		return err
	}

	switch namespaceType {
	case NetworkNamespace:
		err = unix.Setns(nsFd, unix.CLONE_NEWNET)
	case MountNamespace:
		err = unix.Setns(nsFd, unix.CLONE_NEWNS)
	}

	return err
}

func GetOriginNsFd(namespaceType NamespaceType) (int, error) {
	// Save the current network namespace
	pidPath := strconv.Itoa(os.Getpid())
	tidPath := strconv.Itoa(unix.Gettid())
	orignsPathBase := filepath.Join(procDir, pidPath, "/task", tidPath)
	orignsPath := ""

	switch namespaceType {
	case NetworkNamespace:
		orignsPath = filepath.Join(orignsPathBase, "/ns/net")
	case MountNamespace:
		orignsPath = filepath.Join(orignsPathBase, "/ns/mnt")
	}

	origns, err := GetFdFromPath(orignsPath)

	if err != nil {
		return -1, err
	}
	defer unix.Close(origns)

	return origns, nil
}

func GetUniqueNamespaces(namespaceType NamespaceType) (map[uint64]string, error) {
	namespacesFound := make(map[uint64]string)

	procDir := "/proc"
	procContents, err := ioutil.ReadDir(procDir)

	if err != nil {
		return namespacesFound, err
	}

	for _, entry := range procContents {
		if !entry.IsDir() {
			continue
		}
		match, _ := filepath.Match("[1-9]*", entry.Name())
		if match {
			// Check if /proc/PID/net/* exists
			checkPath := ""

			switch namespaceType {
			case NetworkNamespace:
				checkPath = filepath.Join(procDir, entry.Name(), "/ns/net")
			case MountNamespace:
				checkPath = filepath.Join(procDir, entry.Name(), "/ns/mnt")
			}

			if _, err := os.Stat(checkPath); !os.IsNotExist(err) {
				// path for /proc/PID/ns/net exists
				// inode used to track unique namespaces
				var inode uint64

				fileinfo, err := os.Stat(checkPath)

				if err != nil {
					//{{if .Config.Debug}}
					log.Printf("error : %v", err)
					//{{end}}
					continue
				}
				inode = fileinfo.Sys().(*syscall.Stat_t).Ino
				// Track unique namespaces
				namespacesFound[inode] = checkPath
			}

		}
	}
	return namespacesFound, nil
}
