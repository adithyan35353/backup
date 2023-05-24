package jobs

import (
	"gitlab.4medica.net/gke/kube-backup/config"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/sendgrid/go-solr"
	"net"
	"github.com/codeskyblue/go-sh"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"time"
	"github.com/pkg/errors"
	"net/url"
)

type PodToBackup struct {
	BaseURL       string
	podName       string
	podNameSpace  string
	BackupSuccess bool
	ShardName     string
}

type SolrBackupResponse struct {
	Status string `json:"status"`
	ResponseHeader struct {
		Status int `json:"status"`
		QTime  int `json:"QTime"`
	} `json:"response"`
	Exception string `json:"exception"`
}

type SolrReplicaDetailsResponse struct {
	ResponseHeader struct {
		Status int `json:"status"`
		QTime  int `json:"QTime"`
	} `json:"response"`
	Details map[string]interface {
	} `json:"details"`
	Exception string `json:"exception"`
}

func RunSolrBackup(plan config.Plan, tmpPath string, filePostFix string) (string, string, error) {
	backupLocation := fmt.Sprintf("%v/%v-%v", tmpPath, plan.Name, filePostFix)
	archive := backupLocation + ".tgz"
	log := fmt.Sprintf("%v/%v-%v.log", tmpPath, plan.Name, filePostFix)
	zkHost := plan.Target["zkHost"].(string);
	solrCollection := plan.Target["collection"].(string)
	var remoteBackupLocation = "/tmp";

	if val, ok := plan.Target["remoteBackupLocation"]; ok {
		remoteBackupLocation = val.(string);
	}

	remoteBackupName := filePostFix

	state, err := getClusterState(zkHost, solrCollection)

	if err != nil {
		return archive, log, err
	}

	solrHostPostFix := plan.Target["solrHostPostFix"].(string)
	podsToBackup, err := retrievePodsToBackup(state.Collections[solrCollection].Shards, solrHostPostFix)

	logrus.WithField("plan", plan.Name).Info("Pods for backup is decided ", podsToBackup)

	if (err != nil) {
		return archive, log, err
	}

	httpClient := &http.Client{}
	backupTimeout := time.Duration(plan.Scheduler.Timeout) * time.Minute

	for _, pod := range podsToBackup {
		t1 := time.Now()

		if (err != nil) {
			return archive, log, err
		}

		resp, err := initiateReplicaBackup(httpClient, pod.BaseURL, solrCollection, remoteBackupLocation, remoteBackupName)

		logrus.WithField("plan", plan.Name).Info("Backup initiated for ", pod)

		if err != nil {
			return archive, log, err;
		} else if resp.Exception != "" {
			return archive, log, errors.New(resp.Exception)
		} else {
			status := ""

			for status != "success" {
				if (time.Now().Sub(t1) > backupTimeout) {
					return archive, log, errors.New("Backup execution timedout")
				}

				time.Sleep(5 * time.Second)
				status, err = checkReplicaBackupStatus(httpClient, pod.BaseURL, solrCollection, remoteBackupName)
			}

			logrus.WithField("plan", plan.Name).Info("Solr index snapshot generated successfully for ", pod)

			shardBackupLocation := fmt.Sprintf("%v/%v", backupLocation, pod.ShardName)
			err := retrieveBackup(pod.podName, pod.podNameSpace, shardBackupLocation, remoteBackupLocation, remoteBackupName, log)

			if (err != nil) {
				return archive, log, err;
			}

			logrus.WithField("plan", plan.Name).Info("Retrieved backup for ", pod)

		}
	}

	err = createArchiveAndCleanup(backupLocation, plan.Name, log)

	return archive, log, err
}

func getClusterState(host, collection string) (solr.ClusterState, error) {
	var solrzk = solr.NewSolrZK(host, "", collection)
	var err = solrzk.Listen()

	defer solrzk.StopListeningAndCloseConnection()
	if err != nil {
		return solr.ClusterState{}, err
	} else {
		return solrzk.GetClusterState()
	}
}

func retrievePodsToBackup(shards map[string]solr.Shard, solrHostPostFix string) ([]PodToBackup, error) {
	podsToBackup := make([]PodToBackup, 0)

	for _, shard := range shards {
		if shard.State == "active" {
			for _, replica := range shard.Replicas {
				if replica.Leader == "true" && replica.State == "active" {
					baseUrl, err := url.Parse(replica.BaseURL)

					if (err != nil) {
						return podsToBackup, err
					}

					baseUrl.Host = baseUrl.Hostname() + solrHostPostFix + ":" + baseUrl.Port()

					podIps, err := net.LookupIP(baseUrl.Hostname())

					if (err != nil) {
						return podsToBackup, errors.Wrapf(err, "IP lookup failed for host %v", baseUrl.Hostname())
					}

					if len(podIps) > 0 {
						podNameCommand := fmt.Sprintf("kubectl get pod --all-namespaces -o "+
							"jsonpath='{range .items[*]}{.metadata.name} {..podIP} "+
							"{.status.containerStatuses[0].state}{\"\\n\"}{end}' "+
							"--sort-by=.metadata.name|grep running|grep ' %v '|awk '{printf $1}'", podIps[0])
						podNameSpaceCommand := fmt.Sprintf("kubectl get pod --all-namespaces -o "+
							"jsonpath='{range .items[*]}{.metadata.namespace} {..podIP} "+
							"{.status.containerStatuses[0].state}{\"\\n\"}{end}' "+
							"--sort-by=.metadata.namespace|grep running|grep ' %v '|awk '{printf $1}'", podIps[0])

						podName, err1 := sh.Command("/bin/sh", "-c", podNameCommand).Output()
						podNameSpace, err2 := sh.Command("/bin/sh", "-c", podNameSpaceCommand).Output()

						if err1 == nil && err2 == nil {
							podToBackup := PodToBackup{
								baseUrl.String(),
								string(podName),
								string(podNameSpace),
								false,
								shard.Name,
							}

							podsToBackup = append(podsToBackup, podToBackup)
							break;
						}
					} else {
						return podsToBackup, errors.Errorf("Could not find any ip for host %v", baseUrl.Hostname())
					}
				}
			}
		}
	}

	return podsToBackup, nil
}

func initiateReplicaBackup(httpClient *http.Client, nodeUri, collection, backupLocation, backupName string) (SolrBackupResponse, error) {
	backupUrl := fmt.Sprintf("%s/%s/replication?command=backup&wt=json&location=%s&name=%s",
		nodeUri, collection, backupLocation, backupName)
	req, err := http.NewRequest("GET", backupUrl, nil)
	var sr SolrBackupResponse

	if err != nil {
		return sr, err
	}

	req.Header.Add("Content-Type", "application/json")
	resp, err := httpClient.Do(req)

	if err != nil {
		return sr, err
	}

	if resp.StatusCode != 200 {
		htmlData, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return sr, err
		}

		if resp.StatusCode < 500 {
			return sr, solr.NewSolrError(resp.StatusCode, string(htmlData))
		} else {
			return sr, solr.NewSolrInternalError(resp.StatusCode, string(htmlData))
		}
	}

	dec := json.NewDecoder(resp.Body)

	return sr, dec.Decode(&sr)
}

func checkReplicaBackupStatus(httpClient *http.Client, nodeUri, collection, backupName string) (string, error) {
	replicaDetailsUrl := fmt.Sprintf("%s/%s/replication?command=details&wt=json", nodeUri, collection)
	req, err := http.NewRequest("GET", replicaDetailsUrl, nil)
	status := ""
	var sr SolrReplicaDetailsResponse

	if err != nil {
		return status, err
	}

	resp, err := httpClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		htmlData, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return status, err
		}

		if resp.StatusCode < 500 {
			return status, solr.NewSolrError(resp.StatusCode, string(htmlData))
		} else {
			return status, solr.NewSolrInternalError(resp.StatusCode, string(htmlData))
		}
	}

	dec := json.NewDecoder(resp.Body)

	err = dec.Decode(&sr)

	if val, ok := sr.Details["backup"]; ok {
		status = val.([]interface{})[5].(string)
		backupNameFromResponse := val.([]interface{})[9].(string)

		if backupNameFromResponse != backupName {
			status = "InProgress"
		}
	}

	return status, err
}

func retrieveBackup(podName, podNameSpace, backupLocation, remoteBackupLocation, remoteBackupName, logFile string) error {
	careateBkpDirCmd := fmt.Sprintf("mkdir -p %v", backupLocation)
	backupCopyCmd := fmt.Sprintf("kubectl -n %v cp %v:%v/snapshot.%v %v", podNameSpace, podName,
		remoteBackupLocation, remoteBackupName, backupLocation)
	backupRemoteCleanCmd := fmt.Sprintf("kubectl -n %v exec -i %v -- sh -c \"rm -rf %v/snapshot.%v\"", podNameSpace, podName,
		remoteBackupLocation, remoteBackupName)

	commandOutput, err := sh.Command("/bin/sh", "-c", careateBkpDirCmd).CombinedOutput()
	logToFile(logFile, commandOutput)

	if err != nil {
		return err
	} else {
		commandOutput, err = sh.Command("/bin/sh", "-c", backupCopyCmd).CombinedOutput()
		logToFile(logFile, commandOutput)

		// cleanup
		if sh.Command("/bin/sh", "-c", backupRemoteCleanCmd).Run() != nil {
			// show warning
		}

		return err;
	}
}
