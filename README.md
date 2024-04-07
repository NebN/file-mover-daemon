# File mover daemon
Customize `conf/conf.yml`

```yaml
folders:
  - source: /source/folder/one
    destination: /destination/folder/one
  - source: /source/folder/two
    destination: /destination/folder/two
```

And run 
```
go run cmd/main.go
```
