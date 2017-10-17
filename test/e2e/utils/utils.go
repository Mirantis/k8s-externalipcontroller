// Copyright 2016 Mirantis
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiremotecommand "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	_ "k8s.io/client-go/pkg/api/install"
	"k8s.io/client-go/pkg/api/v1"
	v1beta1 "k8s.io/client-go/pkg/apis/rbac/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
)

var (
	MASTER     string
	TESTLINK   string
	MASTERNAME string
)

func init() {
	flag.StringVar(&MASTER, "master", "http://apiserver:8888", "apiserver address to use with restclient")
	flag.StringVar(&TESTLINK, "testlink", "eth0", "link to use on the side of tests")
	flag.StringVar(&MASTERNAME, "mastername", "kube-master", "node that wont be used for scheduling")
}

func GetTestLink() string {
	return TESTLINK
}

func Logf(format string, a ...interface{}) {
	fmt.Fprintf(GinkgoWriter, format, a...)
}

func LoadConfig() *rest.Config {
	config, err := clientcmd.BuildConfigFromFlags(MASTER, "")
	Expect(err).NotTo(HaveOccurred())
	return config
}

func KubeClient() (*kubernetes.Clientset, error) {
	Logf("Using master %v\n", MASTER)
	config := LoadConfig()
	clientset, err := kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred())
	return clientset, nil
}

func WaitForReady(clientset *kubernetes.Clientset, pod *v1.Pod) {
	Eventually(func() error {
		podUpdated, err := clientset.Core().Pods(pod.Namespace).Get(pod.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if podUpdated.Status.Phase != v1.PodRunning {
			return fmt.Errorf("pod %v is not running phase: %v", podUpdated.Name, podUpdated.Status.Phase)
		}
		return nil
	}, 120*time.Second, 5*time.Second).Should(BeNil())
}

func DumpLogs(clientset *kubernetes.Clientset, pods ...v1.Pod) {
	for _, pod := range pods {
		dumpLogs(clientset, pod)
	}
}

func dumpLogs(clientset *kubernetes.Clientset, pod v1.Pod) {
	req := clientset.Core().Pods(pod.Namespace).GetLogs(pod.Name, &v1.PodLogOptions{})
	readCloser, err := req.Stream()
	Expect(err).NotTo(HaveOccurred())
	defer readCloser.Close()
	Logf("\n Dumping logs for %v:%v \n", pod.Namespace, pod.Name)
	_, err = io.Copy(GinkgoWriter, readCloser)
	Expect(err).NotTo(HaveOccurred())
}

func ExecInPod(clientset *kubernetes.Clientset, pod v1.Pod, cmd ...string) (string, string, error) {
	Logf("Running %v in %v\n", cmd, pod.Name)
	container := pod.Spec.Containers[0].Name
	var stdout, stderr bytes.Buffer
	config := LoadConfig()
	rest := clientset.Core().RESTClient()
	req := rest.Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		Param("container", container)
	req.VersionedParams(&api.PodExecOptions{
		Container: container,
		Command:   cmd,
		TTY:       false,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
	}, api.ParameterCodec)
	err := execute("POST", req.URL(), config, nil, &stdout, &stderr, false)
	Logf("Error %v: %v\n", cmd, stderr.String())
	Logf("Output %v: %v\n", cmd, stdout.String())
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

func execute(method string, url *url.URL, config *rest.Config, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
	exec, err := remotecommand.NewExecutor(config, method, url)
	if err != nil {
		return err
	}
	return exec.Stream(remotecommand.StreamOptions{
		SupportedProtocols: []string{
			apiremotecommand.StreamProtocolV4Name,
			apiremotecommand.StreamProtocolV3Name,
			apiremotecommand.StreamProtocolV2Name,
			apiremotecommand.StreamProtocolV1Name,
		},
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    tty,
	})
}

// AddServiceAccountToAdmins will add system:serviceaccounts to cluster-admin ClusterRole
func AddServiceAccountToAdmins(c kubernetes.Interface) {
	By("Adding service account group to cluster-admin role")
	roleBinding := &v1beta1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system:serviceaccount-admin",
		},
		Subjects: []v1beta1.Subject{{
			Kind: "Group",
			Name: "system:serviceaccounts",
		}},
		RoleRef: v1beta1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	_, err := c.Rbac().ClusterRoleBindings().Create(roleBinding)
	if !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred(), "Failed to create role binding for serviceaccounts")
	}
}
