package homedir

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// DisableCache will disable caching of the home directory. Caching is enabled
// by default.
var DisableCache bool

var homedirCache string
var userCache string
var whoamiBypass bool
var cacheLock sync.RWMutex

// User returns the executing user name.
//
// This uses an OS-specific method for discovering the user name.
// An error is returned if the user name cannot be detected.
func User() (string, error) {
	if !DisableCache {
		cacheLock.RLock()
		cached := userCache
		cacheLock.RUnlock()
		if cached != "" {
			return cached, nil
		}
	}

	cacheLock.Lock()
	defer cacheLock.Unlock()

	var result string
	var err error
	if runtime.GOOS == "windows" {
		result, err = userWindows()
	} else {
		// Unix-like system, so just assume Unix
		result, err = userUnix()
	}

	if err != nil {
		return "", err
	}
	userCache = result
	return result, nil
}

// Dir returns the home directory for the executing user.
//
// This uses an OS-specific method for discovering the home directory.
// An error is returned if a home directory cannot be detected.
func Dir() (string, error) {
	if !DisableCache {
		cacheLock.RLock()
		cached := homedirCache
		cacheLock.RUnlock()
		if cached != "" {
			return cached, nil
		}
	}

	cacheLock.Lock()
	defer cacheLock.Unlock()

	var result string
	var err error
	if runtime.GOOS == "windows" {
		result, err = dirWindows()
	} else {
		// Unix-like system, so just assume Unix
		result, err = dirUnix()
	}

	if err != nil {
		return "", err
	}
	homedirCache = result
	return result, nil
}

func userUnix() (string, error) {
	// First prefer the USER environmental variable
	if user := os.Getenv("USER"); user != "" {
		return user, nil
	}

	// If that fails, try whoami
	var stdout bytes.Buffer
	cmd := exec.Command("whoami")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		// If "whoami" is missing, ignore it
		if err == exec.ErrNotFound {
			return "", err
		}
	} else {
		result := strings.TrimSpace(stdout.String())
		if result != "" && !whoamiBypass {
			return result, nil
		}
	}

	// try id
	cmd = exec.Command("id")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		// If "id" is missing, ignore it
		if err == exec.ErrNotFound {
			return "", err
		}
	}

	r, err := regexp.Compile("uid=\\d+\\((\\w+)\\)")
	if err != nil {
		return "", fmt.Errorf("exhausted methods to obtain username")
	}
	sm := r.FindStringSubmatch(stdout.String())
	if len(sm) != 2 {
		return "", fmt.Errorf("exhausted methods to obtain username")
	}

	return sm[1], nil
}

func userWindows() (string, error) {
	// First prefer the USER environmental variable
	if user := os.Getenv("USERNAME"); user != "" {
		return user, nil
	}

	return "", fmt.Errorf("exhausted methods to obtain username")
}

// Expand expands the path to include the home directory if the path
// is prefixed with `~`. If it isn't prefixed with `~`, the path is
// returned as-is.
func Expand(path string) (string, error) {
	if len(path) == 0 {
		return path, nil
	}

	if path[0] != '~' {
		return path, nil
	}

	if len(path) > 1 && path[1] != '/' && path[1] != '\\' {
		return "", errors.New("cannot expand user-specific home dir")
	}

	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, path[1:]), nil
}

func dirUnix() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	// If that fails, try getent
	var stdout bytes.Buffer
	cmd := exec.Command("getent", "passwd", strconv.Itoa(os.Getuid()))
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		// If "getent" is missing, ignore it
		if err == exec.ErrNotFound {
			return "", err
		}
	} else {
		if passwd := strings.TrimSpace(stdout.String()); passwd != "" {
			// username:password:uid:gid:gecos:home:shell
			passwdParts := strings.SplitN(passwd, ":", 7)
			if len(passwdParts) > 5 {
				return passwdParts[5], nil
			}
		}
	}

	// If all else fails, try the shell
	stdout.Reset()
	cmd = exec.Command("sh", "-c", "cd && pwd")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		return "", errors.New("blank output when reading home directory")
	}

	return result, nil
}

func dirWindows() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	drive := os.Getenv("HOMEDRIVE")
	path := os.Getenv("HOMEPATH")
	home := drive + path
	if drive == "" || path == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		return "", errors.New("HOMEDRIVE, HOMEPATH, and USERPROFILE are blank")
	}

	return home, nil
}
