package main

import (
	"flag"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/Sirupsen/logrus"
	"gitlab.4medica.net/gke/kube-backup/api"
	"gitlab.4medica.net/gke/kube-backup/config"
	"gitlab.4medica.net/gke/kube-backup/db"
	"gitlab.4medica.net/gke/kube-backup/scheduler"
	"gitlab.4medica.net/gke/kube-backup/backup"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var version = "undefined"

func main() {
	var appConfig = &config.AppConfig{}
	flag.StringVar(&appConfig.LogLevel, "LogLevel", "debug", "logging threshold level: debug|info|warn|error|fatal|panic")
	flag.IntVar(&appConfig.Port, "Port", 8090, "HTTP port to listen on")
	flag.StringVar(&appConfig.ConfigPath, "ConfigPath", "/kube-backup/config", "plan yml files dir")
	flag.StringVar(&appConfig.StoragePath, "StoragePath", "/kube-backup/data/storage", "backup storage")
	flag.StringVar(&appConfig.TmpPath, "TmpPath", "/tmp", "temporary backup storage")
	flag.StringVar(&appConfig.DataPath, "DataPath", "/kube-backup/data/db", "db dir")
	flag.Parse()
	setLogLevel(appConfig.LogLevel)
	logrus.Infof("Starting with config: %+v", appConfig)

	verifyApplicationEnvironment()

	plans, err := config.LoadPlans(appConfig.ConfigPath)

	if err != nil {
		logrus.Fatal(err)
	}

	err = os.MkdirAll(appConfig.DataPath, 0755)

	if err != nil {
		logrus.Fatal(err)
	}

	store, err := db.Open(path.Join(appConfig.DataPath, "kubd-db-backup.db"))

	if err != nil {
		logrus.Fatal(err)
	}

	statusStore, err := db.NewStatusStore(store)

	if err != nil {
		logrus.Fatal(err)
	}

	sch := scheduler.New(plans, appConfig, statusStore)
	err = sch.Start()

	if err != nil {
		logrus.Fatal(err)
	}

	server := &api.HttpServer{
		Config: appConfig,
		Stats:  statusStore,
	}
	logrus.Infof("Starting HTTP server on port %v", appConfig.Port)
	go server.Start(version)

	//wait for SIGINT (Ctrl+C) or SIGTERM (docker stop)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logrus.Infof("Shutting down %v signal received", sig)
}

func verifyApplicationEnvironment() {
	if (config.AppEnv == "production") {
		info, err := backup.CheckMongodump()

		if err != nil {
			logrus.Fatal(err)
		}

		logrus.Info(info)

		info, err = backup.CheckMongoClient()

		if err != nil {
			logrus.Fatal(err)
		}

		logrus.Info(info)

		info, err = backup.CheckMinioClient()

		if err != nil {
			logrus.Fatal(err)
		}

		logrus.Info(info)

		info, err = backup.CheckGCloudClient()

		if err != nil {
			logrus.Fatal(err)
		}

		logrus.Info(info)

		// @todo: azure is diabled for now
		//info, err = backup.CheckAzureClient()
		//
		//if err != nil {
		//	logrus.Fatal(err)
		//}
		//
		//logrus.Info(info)

		info, err = backup.CheckKubeClient()

		if err != nil {
			logrus.Fatal(err)
		}

		logrus.Info(info)
	}
}

func setLogLevel(levelName string) {
	level, err := logrus.ParseLevel(levelName)

	if err != nil {
		logrus.Fatal(err)
	}

	logrus.SetLevel(level)
}
