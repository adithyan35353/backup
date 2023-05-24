package jobs

import (
	"gitlab.4medica.net/gke/kube-backup/config"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"gopkg.in/yaml.v2"
	"github.com/pkg/errors"
	"github.com/codeskyblue/go-sh"
	"strconv"
	"strings"
	"time"
	"github.com/Sirupsen/logrus"
)

type mongoPurgeCollection [] struct {
	Name           string  `yaml:"name"`
	TimestampField string  `yaml:"timestampField"`
	ThresholdSize  float64 `yaml:"thresholdSize"`
	PurgeOlderThan int `yaml:"purgeOlderThan"`
}

func RunMongoPurge(plan config.Plan, k8Client *kubernetes.Clientset, tmpPath string, filePostFix string) (string, string, error) {
	var collectionsToBePurged mongoPurgeCollection
	backupLocation := fmt.Sprintf("%v/%v-%v", tmpPath, plan.Name, filePostFix)
	var archive string
	log := fmt.Sprintf("%v/%v-%v.log", tmpPath, plan.Name, filePostFix)
	k8coreV1Client := k8Client.CoreV1();
	username, password, err := retrieveMongoCredentialsFromSecret(k8coreV1Client, plan.Target["secret"])
	timeout := plan.Scheduler.Timeout

	if err != nil {
		return "", log, errors.Wrap(err, "Failed to retrieve credentials")
	}

	collections, err := parseCollectionsFromTarget(plan.Target["collections"])

	if err != nil {
		return "", log, errors.Wrap(err, "Failed to parse collections")
	}

	mongoBaseCommand := formMongoBaseCommand(plan.Target, username, password)


	for _, collection := range collections {
		sizeInGB, err := retrieveCollectionSize(mongoBaseCommand, collection.Name, log)

		if err != nil {
			return "", log, err
		}

		if sizeInGB > collection.ThresholdSize {
			collectionsToBePurged = append(collectionsToBePurged, collection)
		}
	}

	mongoBackupCommand := formMongoBackupCommand(plan.Target, username, password, backupLocation)

	for _, collection := range collectionsToBePurged {
		if (plan.Target["backupBeforePurge"].(bool)) {
			err = backupCollection(mongoBackupCommand, collection.Name, log, timeout)

			if err != nil {
				return "", log, err
			}
		}

		dateString := time.Now().AddDate(0, 0, -collection.PurgeOlderThan).Format("2006-01-02")
		err = purgeCollection(mongoBaseCommand, collection.Name, collection.TimestampField, dateString, log, timeout)

		if err != nil {
			return "", log, err
		}
	}
	
	if len(collectionsToBePurged) > 0 && plan.Target["backupBeforePurge"].(bool) {
		err = createArchiveAndCleanup(backupLocation, plan.Name, log)
		archive = backupLocation + ".tgz"
	} 

	return archive, log, err
}

func retrieveCollectionSize(baseCmd string, collectionName string, logFile string) (float64, error) {
	collectionSizeCommand := fmt.Sprintf("%v --quiet --eval 'db[`'%v'`].stats().size'", baseCmd, collectionName)
	var emptyFloat float64
	var size int

	output, err := sh.Command("sh", "-c", collectionSizeCommand).CombinedOutput()
	outputString := strings.TrimSuffix(string(output), "\n")

	if err != nil {
		logToFile(logFile, output)

		return emptyFloat, errors.Wrap(err, "Unable to find collection size")
	} else if outputString == "" {
		return emptyFloat, errors.New("Unable to find collection size")
	}

	// https://jira.mongodb.org/browse/SERVER-27159
	outputStringSlice := strings.Split(outputString, "\n")
	size, err = strconv.Atoi(outputStringSlice[len(outputStringSlice)-1])

	if err != nil {
		return emptyFloat, errors.Wrap(err, "Unable to find collection size")
	}

	sizeInGB := float64(size) / 1024 / 1024 / 1024

	logrus.WithFields(logrus.Fields{
		"collection": collectionName,
		"size": sizeInGB,
	}).Debug("Successfully retrieved collection size")


	return sizeInGB, nil
}

func formMongoBaseCommand (target map[string]interface{}, username string, password string) string {
	mongoBaseCommand := fmt.Sprintf("mongo --host %v %v ",
		target["connectionString"].(string),
		target["database"].(string),
	)

	if username != "" && password != "" {
		mongoBaseCommand += fmt.Sprintf("-u %v -p %v ", username, password)
	}

	if target["params"].(string) != "" {
		mongoBaseCommand += fmt.Sprintf("%v", target["params"].(string))
	}

	return mongoBaseCommand
}

func formMongoBackupCommand (target map[string]interface{}, username string, password string, backupLocation string) string {
	mongoBaseCommand := fmt.Sprintf("mongodump --host %v --db %v --out %v ",
		target["connectionString"].(string),
		target["database"].(string),
		backupLocation,
	)

	if username != "" && password != "" {
		mongoBaseCommand += fmt.Sprintf("-u %v -p %v ", username, password)
	}

	if target["params"].(string) != "" {
		mongoBaseCommand += fmt.Sprintf("%v", target["params"].(string))
	}

	return mongoBaseCommand
}

func parseCollectionsFromTarget (collections interface{}) (mongoPurgeCollection, error) {
	parsedCollections := make(mongoPurgeCollection, 0)
	collectionsString, err := yaml.Marshal(collections)

	if err != nil {
		return parsedCollections, err
	}

	err = yaml.Unmarshal(collectionsString, &parsedCollections)

	return parsedCollections, err;
}

func backupCollection (baseCmd, collection, logFile string, timeout int) error {
	collectionBackupCommand := fmt.Sprintf("%v --collection %v ", baseCmd, collection)

	output, err := sh.Command("/bin/sh", "-c", collectionBackupCommand).SetTimeout(time.Duration(timeout) * time.Minute).CombinedOutput()
	if err != nil {
		ex := ""
		if len(output) > 0 {
			ex = strings.Replace(string(output), "\n", " ", -1)
		}

		return errors.Wrapf(err, "mongodump log %v", ex)
	}

	logToFile(logFile, output)
	logrus.WithFields(logrus.Fields{
		"collection": collection,
	}).Debug("Collection backup successful")

	return nil
}

func purgeCollection (baseCmd, collection, timestampField, dateString, logFile string, timeout int) error {
	collectionPurgeCommand := fmt.Sprintf("%v --quiet --eval " +
		"'db.%v.remove({\"%v\" : { $lte :  ISODate(\"%v\") }})'", baseCmd, collection, timestampField, dateString)

	output, err := sh.Command("/bin/sh", "-c", collectionPurgeCommand).SetTimeout(time.Duration(timeout) * time.Minute).CombinedOutput()

	if err != nil {
		ex := ""
		if len(output) > 0 {
			ex = strings.Replace(string(output), "\n", " ", -1)
		}

		return errors.Wrapf(err, "mongopurge log %v", ex)
	}

	logToFile(logFile, output)
	logrus.WithFields(logrus.Fields{
		"collection": collection,
	}).Debug("Successfully purged collection")

	return nil
}
