package jobs

import (
	"gitlab.4medica.net/gke/kube-backup/config"
	"time"
	"fmt"
	"github.com/codeskyblue/go-sh"
	"strings"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
)

func RunMongoBackup(plan config.Plan, k8Client *kubernetes.Clientset, tmpPath string, filePostFix string) (string, string, error) {
	archive := fmt.Sprintf("%v/%v-%v.gz", tmpPath, plan.Name, filePostFix)
	log := fmt.Sprintf("%v/%v-%v.log", tmpPath, plan.Name, filePostFix)
	k8coreV1Client := k8Client.CoreV1();

	username, password, err := retrieveMongoCredentialsFromSecret(k8coreV1Client, plan.Target["secret"])

	if err != nil {
		return archive, log, errors.Wrap(err, "Failed to retrieve credentials")
	}

	dump := fmt.Sprintf("mongodump --archive=%v --gzip --host %v --port %v ",
		archive, plan.Target["host"].(string), plan.Target["port"].(string))
	if plan.Target["database"] != "" {
		dump += fmt.Sprintf("--db %v ", plan.Target["database"])
	}

	if username != "" && password != "" {
		dump += fmt.Sprintf("-u %v -p %v ", username, password)
	}

	if plan.Target["params"].(string) != "" {
		dump += fmt.Sprintf("%v", plan.Target["params"].(string))
	}

	output, err := sh.Command("/bin/sh", "-c", dump).SetTimeout(time.Duration(plan.Scheduler.Timeout) * time.Minute).CombinedOutput()

	if err != nil {
		ex := ""
		if len(output) > 0 {
			ex = strings.Replace(string(output), "\n", " ", -1)
		}
		return "", "", errors.Wrapf(err, "mongodump log %v", ex)
	}

	logToFile(log, output)

	return archive, log, nil
}