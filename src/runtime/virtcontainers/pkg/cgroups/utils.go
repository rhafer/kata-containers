// Copyright (c) 2020 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

// prepend a kata specific string to oci cgroup path to
// form a different cgroup path, thus cAdvisor couldn't
// find kata containers cgroup path on host to prevent it
// from grabbing the stats data.
const CgroupKataPrefix = "kata"

// DefaultCgroupPath runtime-determined location in the cgroups hierarchy.
const DefaultCgroupPath = "/vc"

func RenameCgroupPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("Cgroup path is empty")
	}

	cgroupPathDir := filepath.Dir(path)
	cgroupPathName := fmt.Sprintf("%s_%s", CgroupKataPrefix, filepath.Base(path))
	return filepath.Join(cgroupPathDir, cgroupPathName), nil

}

// validCgroupPath returns a valid cgroup path.
// see https://github.com/opencontainers/runtime-spec/blob/master/config-linux.md#cgroups-path
func ValidCgroupPath(path string, systemdCgroup bool) (string, error) {
	if IsSystemdCgroup(path) {
		return path, nil
	}

	if systemdCgroup {
		return "", fmt.Errorf("malformed systemd path '%v': expected to be of form 'slice:prefix:name'", path)
	}

	// In the case of an absolute path (starting with /), the runtime MUST
	// take the path to be relative to the cgroups mount point.
	if filepath.IsAbs(path) {
		return RenameCgroupPath(filepath.Clean(path))
	}

	// In the case of a relative path (not starting with /), the runtime MAY
	// interpret the path relative to a runtime-determined location in the cgroups hierarchy.
	// clean up path and return a new path relative to DefaultCgroupPath
	return RenameCgroupPath(filepath.Join(DefaultCgroupPath, filepath.Clean("/"+path)))
}

func IsSystemdCgroup(cgroupPath string) bool {
	// systemd cgroup path: slice:prefix:name
	re := regexp.MustCompile(`([[:alnum:]]|\.)+:([[:alnum:]]|\.)+:([[:alnum:]]|\.)+`)
	found := re.FindStringIndex(cgroupPath)

	// if found string is equal to cgroupPath then
	// it's a correct systemd cgroup path.
	return found != nil && cgroupPath[found[0]:found[1]] == cgroupPath
}

func DeviceToCgroupDevice(device string) (*configs.Device, error) {
	var st unix.Stat_t
	linuxDevice := configs.Device{
		Allow:       true,
		Permissions: "rwm",
		Path:        device,
	}

	if err := unix.Stat(device, &st); err != nil {
		return nil, err
	}

	devType := st.Mode & unix.S_IFMT

	switch devType {
	case unix.S_IFCHR:
		linuxDevice.Type = 'c'
	case unix.S_IFBLK:
		linuxDevice.Type = 'b'
	default:
		return nil, fmt.Errorf("unsupported device type: %v", devType)
	}

	major := int64(unix.Major(st.Rdev))
	minor := int64(unix.Minor(st.Rdev))
	linuxDevice.Major = major
	linuxDevice.Minor = minor

	linuxDevice.Gid = st.Gid
	linuxDevice.Uid = st.Uid
	linuxDevice.FileMode = os.FileMode(st.Mode)

	return &linuxDevice, nil
}

func DeviceToLinuxDevice(device string) (specs.LinuxDeviceCgroup, error) {
	dev, err := DeviceToCgroupDevice(device)
	if err != nil {
		return specs.LinuxDeviceCgroup{}, err
	}

	return specs.LinuxDeviceCgroup{
		Allow:  dev.Allow,
		Type:   string(dev.Type),
		Major:  &dev.Major,
		Minor:  &dev.Minor,
		Access: dev.Permissions,
	}, nil
}
