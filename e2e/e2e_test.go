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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/parca-dev/parca-agent/pkg/debuginfo"
	pb "github.com/parca-dev/parca/gen/proto/go/parca/query/v1alpha1"
	debuginfopb "github.com/parca-dev/parca/gen/proto/go/parca/debuginfo/v1alpha1"
	profilestorepb "github.com/parca-dev/parca/gen/proto/go/parca/profilestore/v1alpha1"
	metastorepb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
	"github.com/parca-dev/parca/pkg/metastore"
)

func TestGRPC(t *testing.T) {
	conn, err := grpc.Dial("127.0.0.1:7070", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	c := pb.NewQueryServiceClient(conn)
	ctx := context.Background()

	endTime := time.Now()
	startTime := endTime.Add(-5 * time.Minute)

	queryRequest := &pb.QueryRangeRequest{
		Query: `memory:alloc_objects:count:space:bytes{job="default"}`,
		Start: timestamppb.New(startTime),
		End:   timestamppb.New(endTime),
		Limit: 10,
	}

	resp, err := c.QueryRange(ctx, queryRequest)

	fmt.Println(resp.String())

	require.NoError(t, err)
	require.NotEmpty(t, resp.Series)
}

/*
func TestSymbolizer(t *testing.T) {
	conn, err := grpc.Dial("127.0.0.1:7070", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	 := context.Background()

	c := pb.NewQueryServiceClient(conn)

	endTime := time.Now()
	startTime := endTime.Add(-5 * time.Minute)

	queryRequest := &pb.QueryRangeRequest{
		Query: "parca_agent_cpu_samples_count",
		Start: timestamppb.New(startTime),
		End:   timestamppb.New(endTime),
		Limit: 10,
	}

	resp, err := c.QueryRange(ctx, queryRequest)

	cc := profilestorepb.NewProfileStoreServiceClient(conn)

	profileRequest := profilestorepb.WriteRawRequest{
		Series: []*profilestorepb.RawProfileSeries{{
			Labels: resp.Series[0].Labelset,
		}},
	}

	respProfile, err := cc.WriteRaw(ctx, profileRequest)


}
*/

func setup(t *testing.T) (*grpc.ClientConn, *debuginfo.Store, metastore.ProfileMetaStore) {
	t.Helper()

	conn, err := grpc.Dial("127.0.0.1:7070", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	debuginfoClient := debuginfopb.NewDebugInfoServiceClient(conn)
	profileStoreClient := profilestorepb.NewProfileStoreServiceClient(conn)

	return conn, debuginfoClient, profileStoreClient 
}
