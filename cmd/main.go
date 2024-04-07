package main

import (
	"io"
	"os"
	"path"
	"time"
	"path/filepath"
	"gopkg.in/yaml.v3"
	"github.com/fsnotify/fsnotify"
	"log/slog"
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

	var destinationMap = map[string]string {}

	for _, folder := range conf.Folders {
		slog.Info("Adding folder to watcher", "folder", folder.Source, "destination", folder.Destination)
		watcher.Add(folder.Source)
		destinationMap[path.Join(folder.Source)] = folder.Destination
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
					blockUntilUnchanging(event.Name)
					base := filepath.Base(event.Name)
					dir := filepath.Dir(event.Name)
					destination := path.Join(destinationMap[dir], base)
					slog.Info("watcher moving file", "source", event.Name, "destination", destination)
					err := mv(event.Name, destination)
					if err != nil {
						slog.Error("watcher move", "error", err.Error())
					}
					slog.Info("watcher move complete", "source", event.Name, "destination", destination)
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

	select {}
}


type Folder struct {
	Source string `yaml:"source"`
	Destination string `yaml:"destination"`
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
