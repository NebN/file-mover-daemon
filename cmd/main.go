package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

func main() {
	conf, err := readConf()
	if err != nil {
		slog.Error("Unable to read conf", "error", err.Error())
		return
	}
	slog.SetLogLoggerLevel(slog.LevelDebug)
	slog.Debug("main", "conf", conf)
	watcher, err := prepareWatcher()
	if err != nil {
		slog.Error("Unable to spawn watcher", "error", err.Error())
		return
	}
	defer watcher.Close()

	var actionMapLocal = map[string]Action{}
	var actionMapShare = map[string]Action{}

	for _, folder := range conf.Folders {
		if folder.IsShare {
			slog.Debug("NOT adding folder to watcher", "folder", folder.Source, "destination", folder.Destination, "command", folder.Command)
			// OS watcher does not work on shared network folders
			continue
		}
		slog.Info("Adding folder to watcher", "folder", folder.Source, "destination", folder.Destination, "command", folder.Command)
		watcher.Add(folder.Source)
		actionMapLocal[path.Join(folder.Source)] = Action{
			Destination: folder.Destination,
			Command:     folder.Command,
		}
	}

	for _, folder := range conf.Folders {
		if !folder.IsShare {
			slog.Debug("NOT adding folder to polling group", "folder", folder.Source, "destination", folder.Destination, "command", folder.Command)
			// OS watcher does not work on shared network folders
			continue
		}
		slog.Info("Adding folder to polling group", "folder", folder.Source, "destination", folder.Destination, "command", folder.Command)
		actionMapShare[path.Join(folder.Source)] = Action{
			Destination: folder.Destination,
			Command:     folder.Command,
		}
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					// channel closed
					return
				}
				slog.Debug("watcher", "event", event)
				if event.Op&fsnotify.Create == fsnotify.Create {
					slog.Info("File created detected", "filename", event.Name)

					if err := performAction(event.Name, actionMapLocal); err != nil {
						slog.Error("watcher move", "error", err.Error())
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					// channel closed
					return
				}
				slog.Error("watcher", "error", err.Error())
			}
		}
	}()

	for folder, _ := range actionMapShare {
		go func() {
			prevFiles, err := ls(folder)
			if err != nil {
				slog.Error("Error reading directory", "folder", folder, "error", err.Error())
			}

			for {
				time.Sleep(5 * time.Second)

				currentFiles, err := ls(folder)
				if err != nil {
					slog.Error("Error reading directory", "folder", folder, "error", err.Error())
					continue
				}

				for path, info := range currentFiles {
					if _, found := prevFiles[path]; !found {
						fmt.Printf("New file detected: %s (Size: %d bytes)\n", path, info.Size())
						performAction(path, actionMapShare)

					}
				}

				prevFiles = currentFiles
			}

		}()
	}

	select {}
}

type Folder struct {
	Source      string  `yaml:"source"`
	Destination string  `yaml:"destination"`
	IsShare     bool    `yaml:"is_share"`
	Command     *string `yaml:"command"`
}

type Action struct {
	Destination string
	Command     *string
}

type Conf struct {
	Folders []Folder `yaml:"folders"`
}

func readConf() (*Conf, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	execDir := filepath.Dir(execPath)
	confPath := filepath.Join(execDir, "conf", "conf.yml")
	conf := Conf{}
	content, err := os.ReadFile(confPath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(content, &conf)
	if err != nil {
		return nil, err
	}
	return &conf, nil
}

func prepareWatcher() (*fsnotify.Watcher, error) {

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return watcher, nil
}

func performAction(file string, actionMap map[string]Action) error {
	base := filepath.Base(file)
	dir := filepath.Dir(file)
	action := actionMap[dir]
	destination := path.Join(action.Destination, base)

	blockUntilUnchanging(file)

	if action.Command != nil {
		commandSections := strings.Fields(*action.Command)
		commandSections = append(commandSections, file)
		commandName := commandSections[0]
		commandArgs := commandSections[1:]
		slog.Info("Running command", "command", fmt.Sprintf("%v", commandSections))
		if err := command(commandName, commandArgs...); err != nil {
			return err
		}

	}

	if err := mv(file, destination); err != nil {
		return err
	}

	slog.Info("watcher move complete", "source", file, "destination", destination)
	return nil
}

func mv(from string, to string) error {
	err := os.Rename(from, to)
	if err == nil {
		return nil
	} else {
		slog.Warn("Unable to move file by renaming, possibly different drives, will copy and remove instead", "error", err.Error())
	}
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()

	dest, err := os.Create(to)
	if err != nil {
		return err
	}
	defer dest.Close()

	if _, err := io.Copy(dest, src); err != nil {
		return err
	}

	if err := os.Remove(from); err != nil {
		return err
	}

	return nil
}

func blockUntilUnchanging(file string) {
	var size = fileSize(file)

	for {
		time.Sleep(1 * time.Second)
		var newSize = fileSize(file)
		if newSize == size || newSize == 0 {
			return
		} else {
			slog.Debug("file has changed, waiting 1 more second", "file", file)
			size = newSize
		}
	}

}

func fileSize(path string) int64 {
	stat, err := os.Stat(path)
	if err != nil {
		slog.Error("Error while checking size", "error", err.Error())
		return 0
	}
	return stat.Size()
}

func command(commandName string, commandArgs ...string) error {
	cmd := exec.Command(commandName, commandArgs...)
	out, err := cmd.Output()

	slog.Debug("Command", "output", out)

	if err != nil {

		if exitError, ok := err.(*exec.ExitError); ok {
			// Get the exit code from the ExitError
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				fmt.Printf("Command exited with non-zero code: %d\n", status.ExitStatus())
			}
		} else {
			slog.Error("Could not run command", "error", err.Error())
		}

		return err
	}

	return nil
}

func ls(dir string) (map[string]os.FileInfo, error) {
	files := make(map[string]os.FileInfo)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files[path] = info
		}
		return nil
	})
	return files, err
}
