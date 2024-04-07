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
	conf, _ := readConf()
	// slog.SetLogLoggerLevel(slog.LevelDebug)
	slog.Debug("main", "conf", conf)
	watcher, err := prepareWatcher()
	if err != nil {
		slog.Error("error", err.Error())
	}
	defer watcher.Close()
	slog.Info("main", "watcher", watcher)

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
					slog.Info("Waiting 5 seconds before moving")
					time.Sleep(5 * time.Second)
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
				slog.Error("error", err.Error())
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
				conf := Conf{}
				content, err := os.ReadFile("conf/conf.yml")
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
