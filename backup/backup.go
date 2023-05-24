package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/codeskyblue/go-sh"
	"gitlab.4medica.net/gke/kube-backup/config"
	"github.com/pkg/errors"
	"gitlab.4medica.net/gke/kube-backup/backup/jobs"
)

func Run(plan config.Plan, tmpPath string, storagePath string) (Result, error) {
	t1 := time.Now()
	planDir := fmt.Sprintf("%v/%v", storagePath, plan.Name)
	var archive, log string
	var err error
	var filePostFix = formatTimeForFilePostFix(t1.UTC());
	k8Client := GetKubernetesClient()
	res := Result{
		Plan:      plan.Name,
		Timestamp: t1.UTC(),
		Status:    500,
		HandlesBackupData: true,
	}

	switch plan.Type {
	case "mongo":
		archive, log, err = jobs.RunMongoBackup(plan, k8Client, tmpPath, filePostFix)
	case "solr":
		archive, log, err = jobs.RunSolrBackup(plan, tmpPath, filePostFix)
	case "file":
		archive, log, err = jobs.RunFileBackup(plan, tmpPath, filePostFix)

		if(archive == "") {
			res.HandlesBackupData = false
		}
	case "mongo-purge":
		archive, log, err = jobs.RunMongoPurge(plan, k8Client, tmpPath, filePostFix)

		if(archive == "") {
			res.HandlesBackupData = false
		}
	}

	err1 := sh.Command("mkdir", "-p", planDir).Run()
	if err1 != nil {
		return res, errors.Wrapf(err1, "creating dir %v in %v failed", plan.Name, storagePath)
	}

	err1 = sh.Command("mv", log, planDir).Run()
	if err1 != nil {
		logrus.WithField("file", log).WithField("target directory", planDir).Warn("failed to move log file")
	}

	if err != nil {
		return res, err
	}

	if(res.HandlesBackupData) {
		_, res.Name = filepath.Split(archive)

		fi, err := os.Stat(archive)
		if err != nil {
			return res, errors.Wrapf(err, "stat file %v failed", archive)
		}
		res.Size = fi.Size()

		err = sh.Command("mv", archive, planDir).Run()
		if err != nil {
			return res, errors.Wrapf(err, "moving file from %v to %v failed", archive, planDir)
		}

		file := filepath.Join(planDir, res.Name)

		err = UploadToRemoteStorage(plan, file)

		if err != nil {
			return res, err
		}
	}

	if plan.Scheduler.Retention > -1 {
		err = applyRetention(planDir, plan.Scheduler.Retention, plan.Scheduler.LogRetention)

		if err != nil {
			return res, errors.Wrap(err, "retention job failed")
		}
	}

	t2 := time.Now()
	res.Status = 200
	res.Duration = t2.Sub(t1)
	return res, nil
}

func formatTimeForFilePostFix(t time.Time) string {
	return t.Format("20060102150405000")
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func UploadToRemoteStorage(plan config.Plan, file string) error {
	if plan.SFTP != nil {
		sftpOutput, err := sftpUpload(file, plan)
		if err != nil {
			return err
		} else {
			logrus.WithField("plan", plan.Name).Info(sftpOutput)
		}
	}

	if plan.S3 != nil {
		s3Output, err := s3Upload(file, plan)
		if err != nil {
			return err
		} else {
			logrus.WithField("plan", plan.Name).Infof("S3 upload finished %v", s3Output)
		}
	}

	if plan.GCloud != nil {
		gCloudOutput, err := gCloudUpload(file, plan)
		if err != nil {
			return err
		} else {
			logrus.WithField("plan", plan.Name).Infof("GCloud upload finished %v", gCloudOutput)
		}
	}

	if plan.Azure != nil {
		azureOutout, err := azureUpload(file, plan)
		if err != nil {
			return err
		} else {
			logrus.WithField("plan", plan.Name).Infof("Azure upload finished %v", azureOutout)
		}
	}

	return nil
}
