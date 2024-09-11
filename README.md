# File mover daemon
Customize `conf/conf.yml`

```yaml
folders:
  - source: /source/folder/one
    destination: /destination/folder/one

  - source: /source/folder/two
    destination: /destination/folder/two
    is_share: true # a polling method has to be used on network shares
    command: echo # this will result in `echo /source/folder/two/example.txt` before the move
```

Compile and run
```
make build
./file-mover-daemon
```

#### Example Systemd unit file
```
[Unit]
Description=Go File Watcher Daemon
After=network.target

[Service]
Type=simple
ExecStart=/path/to/file-mover-daemon/file-mover-daemon
Restart=on-failure
User=youruser
Group=yourgroup

[Install]
WantedBy=multi-user.target
```
