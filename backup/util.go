package backup

import (
	"github.com/codeskyblue/go-sh"
	"fmt"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"flag"
	"path/filepath"
	"k8s.io/client-go/tools/clientcmd"
	"gitlab.4medica.net/gke/kube-backup/config"
)

//evaluate expression in shell
func ShellEval(expression string) (string, error) {
	bucket, err := sh.Command("sh", "-c", fmt.Sprintf("echo -n %v", expression)).CombinedOutput()

	if err != nil {
		return "", errors.Wrapf(err, "unable to evaluate expression %v", expression)
	}

	return string(bucket), nil
}

func GetKubernetesClient() *kubernetes.Clientset {
	var kflags *string
	var err error
	var kubeconfig *rest.Config
	if (config.AppEnv == "production") {
		// creates the in-cluster config
		kubeconfig, err = rest.InClusterConfig()
	} else {

		if home := homeDir(); home != "" {
			kflags = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kflags = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}

		flag.Parse()

		// use the current context in kubeconfig
		kubeconfig, err = clientcmd.BuildConfigFromFlags("", *kflags)
	}

	if err != nil {
		panic(err.Error())
	}

	// creates the clientset
	k8Client, err := kubernetes.NewForConfig(kubeconfig)

	if err != nil {
		panic(err.Error())
	}

	return k8Client
}
