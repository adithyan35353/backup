package jobs

import (
	"github.com/pkg/errors"
	"fmt"
	"github.com/codeskyblue/go-sh"
	"os"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	k8corev1 "k8s.io/api/core/v1"
	k8clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"github.com/Sirupsen/logrus"
)

func logToFile(file string, data []byte) error {
	if len(data) > 0 {
		file, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return errors.Wrapf(err, "Failed opening file: %s", file)
		}
		defer file.Close()

		_, err = file.Write(data)

		if err != nil {
			return errors.Wrapf(err, "Writing log %v failed", file)
		}
	}

	return nil
}

func createArchiveAndCleanup(path, planName, log string) error {
	logrus.WithField("plan", planName).Info("The archive creation is starting \n")
	archive := path + ".tgz"
	// create archive
	createArchiveCommand := fmt.Sprintf("tar -czf %v -C %v .", archive, path)
	commandOutput, err := sh.Command("/bin/sh", "-c", createArchiveCommand).CombinedOutput()
	logToFile(log, commandOutput)

	if err == nil{
		logrus.WithField("plan", planName).Info("Created archive")
	}

	if os.RemoveAll(path) != nil {
		logrus.WithFields(logrus.Fields{
			"path": path,
		}).Warn("Failed to cleanup directory after archiving")
	}

	return errors.Wrapf(err, "Failed to create archive for path %v", path)
}

func isDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// read in ONLY one file
	_, err = f.Readdir(1)

	// and if the file is EOF... well, the dir is empty.
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

func retrieveMongoCredentialsFromSecret(k8Client k8clientcorev1.CoreV1Interface, secret interface{}) (string, string, error) {
	username := ""
	password := ""
	secretSettings := secret.(map[interface{}]interface{})

	secretName := secretSettings["name"].(string)
	secretNameSpace := secretSettings["namespace"].(string)
	usernmeItem := secretSettings["usernameItem"].(string)
	passwordItem := secretSettings["passwordItem"].(string)

	secret, err := k8Client.Secrets(secretNameSpace).Get(secretName, v1.GetOptions{})
	secretmap := secret.(*k8corev1.Secret)

	if err != nil {
		return username, password, err
	}

	logrus.Debug("Successfully retrieved mongodb credentials from secret")

	return string(secretmap.Data[usernmeItem]), string(secretmap.Data[passwordItem]), err
}
