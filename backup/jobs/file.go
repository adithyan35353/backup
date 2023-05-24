package jobs

import (
	"gitlab.4medica.net/gke/kube-backup/config"
	"fmt"
	"github.com/codeskyblue/go-sh"
	"path/filepath"
	"strings"
	"os"
	"errors"
	"time"
)

func RunFileBackup(plan config.Plan, tmpPath string, filePostFix string) (string, string, error) {
	backupLocation := fmt.Sprintf("%v/%v-%v", tmpPath, plan.Name, filePostFix)
	archive := backupLocation + ".tgz"
	backupLocation = backupLocation
	log := fmt.Sprintf("%v/%v-%v.log", tmpPath, plan.Name, filePostFix)
	backupTimeout := time.Duration(plan.Scheduler.Timeout) * time.Minute

	err := retrieveFiles(plan.Target["podLabels"].(string), plan.Target["namespace"].(string),
		plan.Target["paths"].([]interface{}), backupLocation, log, backupTimeout)

	if (err != nil) {
		return archive, log, err
	}

	isEmpty, _ := isDirEmpty(backupLocation)

	if (isEmpty) {
		archive = "";

		failOnFileNotFound, ok := plan.Target["failOnFileNotFound"]

		if(ok && failOnFileNotFound.(bool)) {
			return archive, log, errors.New("Not able to retrieve any files")
		}
	} else {
		err = createArchiveAndCleanup(backupLocation, plan.Name, log)
	}

	return archive, log, err
}

func retrieveFiles(podLabels, namespace string, filePaths []interface{}, backupLocation, logFile string, backupTimeout time.Duration) error {
	pods, _ := getPods(podLabels, namespace, logFile)

	for _, pod := range pods {
		retrieveFileCommandPart := fmt.Sprintf("kubectl -n %v cp %v:", namespace, pod)
		backupLocationForPod := backupLocation + "/" + pod + "/"
		os.MkdirAll(backupLocationForPod, 0755)

		for _, path := range filePaths {
			path, _ := sh.Command("sh", "-c", fmt.Sprintf("echo -n %v", path.(string))).CombinedOutput()

			retrieveFileCommand := fmt.Sprintf(retrieveFileCommandPart+"%v %v%v", string(path), backupLocationForPod, filepath.Base(string(path)))

			output, err := sh.Command("sh", "-c", retrieveFileCommand).SetTimeout(backupTimeout).CombinedOutput()

			if err != nil {
				logToFile(logFile, []byte(err.Error()))
			}

			logToFile(logFile, output)
		}

		isEmpty, _ := isDirEmpty(backupLocationForPod)

		if isEmpty {
			os.Remove(backupLocationForPod)
		}
	}

	return nil;
}

func getPods(podLabels, namespace, logFile string) ([]string, error) {
	labelsArray := strings.Split(strings.TrimSpace(podLabels), " ")
	listPodCommands := fmt.Sprintf("kubectl -n %v get pods -o go-template --template '{{range .items}}"+
		"{{.metadata.name}}{{\" \"}}{{end}}'", namespace)

	for _, label := range labelsArray {
		listPodCommands = fmt.Sprintf(listPodCommands+" -l %v", label)
	}

	output, err := sh.Command("sh", "-c", listPodCommands).CombinedOutput()

	if err != nil {
		return nil, err;
	}

	outputString := string(output)

	if (len(outputString) == 0) {
		return make([]string, 0), nil
	}

	return strings.Split(strings.TrimSpace(outputString), " "), nil
}
