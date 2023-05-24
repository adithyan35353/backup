# kube-backup

This project is extended from https://github.com/stefanprodan/kubebackup/

It provides following types of backup facility for pods running in a kubernetes cluster.

 - MongoDB data
 - MongoDB collection backup an purge
 - Solr data
 - Files or directories from pods

## Configuration

For sample configuration files see test/config

## Web API
- kube-backup-host:8090/storage file server
- kube-backup-host:8090/status backup jobs status
- kube-backup-host:8090/metrics Prometheus endpoint
- kube-backup-host:8090/version kubebackup version and runtime info
- kube-backup-host:8090/debug pprof endpoint

### On demand backup:

HTTP POST kube-backup-host:8090/backup/:planID

```bash
curl -X POST http://kube-backup-host:8090/backup/mongo
```

```json
{
  "plan": "mongo",
  "file": "mongo-1494256295.gz",
  "duration": "3.635186255s",
  "size": "455 kB",
  "timestamp": "2017-05-08T15:11:35.940141701Z"
}
```

### Scheduler status:

- HTTP GET kube-backup-host:8090/status
- HTTP GET kube-backup-host:8090/status/:planID

```bash
curl -X GET http://kube-backup-host:8090/status/mongo
```

```json
{
  "plan": "mongo",
  "next_run": "2017-05-13T14:32:00+03:00",
  "last_run": "2017-05-13T11:31:00.000622589Z",
  "last_run_status": "200",
  "last_run_log": "Backup finished in 2.339055539s archive mongo-1494675060.gz size 527 kB"
}
```

## Build

The project can be built by running command `make build`

## Unit tests

The project do have unit test configuration files, but I haven't configured it to work with new changes.

