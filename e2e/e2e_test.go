// Copyright 2022 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"context"
	"flag"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/query/v1alpha1"
)

var kubeconfig = flag.String("kubeconfig", "~/.kube/config", "kube config path")

//Checks for parca-server and parca-agent pods and returns pod names if true
//Returns empty string if no pods are found
func CheckPodsExist(ctx context.Context, kubeClient kubernetes.Interface) (string, string, error) {
	labelSelectorParcaServer := labels.FormatLabels(map[string]string{"app.kubernetes.io/name": "parca"})
	labelSelectorParcaAgent := labels.FormatLabels(map[string]string{"app.kubernetes.io/name": "parca-agent"})

	parcaServerPod, err := kubeClient.CoreV1().Pods("parca").List(ctx, metav1.ListOptions{LabelSelector: labelSelectorParcaServer})
	parcaAgentPod, err := kubeClient.CoreV1().Pods("parca").List(ctx, metav1.ListOptions{LabelSelector: labelSelectorParcaAgent})

	if err != nil {
		return "", "", fmt.Errorf("Unable to fetch pods in parca namespace: %s", err)
	}

	if len(parcaServerPod.Items) == 0 {
		fmt.Printf("Parca Server Pod not found")
		return "", "", nil
	}

	if len(parcaAgentPod.Items) == 0 {
		fmt.Printf("Parca Agent Pod not found")
		return "", "", nil
	}

	return parcaServerPod.Items[0].Name, parcaAgentPod.Items[0].Name, nil
}

func TestConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := GetKubeConfig(*kubeconfig)
	require.NoError(t, err)

	kubeClient, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)

	parcaServer, parcaAgent, err := CheckPodsExist(ctx, kubeClient)
	if err != nil {
		t.Log(fmt.Errorf("pod discovery error: %s", err))
		require.NoError(t, err)
	}
	fmt.Println("Pods discovered: ", parcaServer, parcaAgent)

	ns := "parca"

	closerServer, err := StartPortForward(ctx, cfg, "https", parcaServer, ns, "7070")
	if err != nil {
		pollErr := fmt.Errorf("failed to start port forwarding Parca Server: %v", err)
		t.Log(pollErr)
		require.NoError(t, err)
	}
	defer closerServer()

	closerAgent, err := StartPortForward(ctx, cfg, "https", parcaAgent, ns, "7071")
	if err != nil {
		pollErr := fmt.Errorf("failed to start port forwarding Parca Agent: %v", err)
		t.Log(pollErr)
		require.NoError(t, err)
	}
	defer closerAgent()

	println("Starting tests")
	conn, err := grpc.Dial("127.0.0.1:7070", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	println("Creating query service client")
	c := pb.NewQueryServiceClient(conn)
	//ctx := context.Background()

	println("Performing Query Range Request")
	queryRequestAgent := &pb.QueryRangeRequest{
		Query: `parca_agent_cpu:samples:count:cpu:nanoseconds:delta`,
		Start: timestamppb.New(timestamp.Time(0)),
		End:   timestamppb.New(timestamp.Time(math.MaxInt64)),
		Limit: 10,
	}

	for i := 0; i < 10; i++ {
		resp1, err1 := c.QueryRange(ctx, queryRequestAgent)

		if err1 != nil {
			status, ok := status.FromError(err1)
			if ok && status.Code() == codes.Unavailable {
				t.Log("query range api unavailable, retrying in a second")
				time.Sleep(time.Minute)
				continue
			}
			t.Fatal(err1)
		}

		//require.NoError(t, err1)
		require.NotEmpty(t, resp1.Series)
	}

	/*	conn, err := grpc.Dial("127.0.0.1:7070", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()

		_, err = http.NewRequestWithContext(ctx, "GET", "https://localhost:9090", nil)
		if err != nil {
			require.NoError(t, err)
		}
	*/
}

func gTestIntegrationGRPC(t *testing.T) {
	println("starting tests")
	conn, err := grpc.Dial("127.0.0.1:7070", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	println("Creating query service client")
	c := pb.NewQueryServiceClient(conn)
	ctx := context.Background()

	println("Performing Query Range Request")
	queryRequestAgent := &pb.QueryRangeRequest{
		Query: `parca_agent_cpu:samples:count:cpu:nanoseconds:delta`,
		Start: timestamppb.New(timestamp.Time(0)),
		End:   timestamppb.New(timestamp.Time(math.MaxInt64)),
		Limit: 10,
	}

	for i := 0; i < 10; i++ {
		resp1, err1 := c.QueryRange(ctx, queryRequestAgent)

		if err1 != nil {
			status, ok := status.FromError(err1)
			if ok && status.Code() == codes.Unavailable {
				t.Log("query range api unavailable, retrying in a second")
				time.Sleep(time.Minute)
				continue
			}
			t.Fatal(err1)
		}

		//require.NoError(t, err1)
		require.NotEmpty(t, resp1.Series)
	}

}
