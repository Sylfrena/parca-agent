// Copyright 2021 The Parca Authors
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

package agent

import (
	"context"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	profilestorepb "github.com/parca-dev/parca/gen/proto/go/parca/profilestore/v1alpha1"
	"google.golang.org/grpc"
)

type Batcher struct {
	logger        log.Logger
	writeClient   profilestorepb.ProfileStoreServiceClient
	writeInterval time.Duration

	mtx    *sync.RWMutex
	series []*profilestorepb.RawProfileSeries

	lastBatchSentAt    time.Time
	lastBatchSendError error
}

func NewBatchWriteClient(logger log.Logger, wc profilestorepb.ProfileStoreServiceClient, writeInterval time.Duration) *Batcher {
	return &Batcher{
		logger:        logger,
		writeClient:   wc,
		writeInterval: writeInterval,

		series: []*profilestorepb.RawProfileSeries{},
		mtx:    &sync.RWMutex{},
	}
}

func (b *Batcher) loopReport(lastBatchSentAt time.Time, lastBatchSendError error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	b.lastBatchSentAt = lastBatchSentAt
	b.lastBatchSendError = lastBatchSendError
}

func (b *Batcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(b.writeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		b.loopReport(time.Now(), b.batchLoop(ctx))
	}
}

func (b *Batcher) batchLoop(ctx context.Context) error {
	b.mtx.Lock()
	batch := b.series
	b.series = []*profilestorepb.RawProfileSeries{}
	b.mtx.Unlock()

	expbackOff := backoff.NewExponentialBackOff()
	expbackOff.MaxElapsedTime = b.writeInterval         // TODO: Subtract ~10% of interval to account for overhead in loop
	expbackOff.InitialInterval = 500 * time.Millisecond // Let's not retry to aggressively to start with.

	err := backoff.Retry(func() error {
		_, err := b.writeClient.WriteRaw(ctx, &profilestorepb.WriteRawRequest{Series: batch})
		// Only log error if retrying, otherwise it will be logged outside the retry
		if err != nil && expbackOff.NextBackOff().Nanoseconds() > 0 {
			level.Debug(b.logger).Log(
				"msg", "batch write client failed to send profiles",
				"retry", expbackOff.NextBackOff(),
				"count", len(batch),
				"err", err,
			)
		}
		return err
	}, expbackOff)
	if err != nil {
		// TODO: Add metric and increase with every backoff iteration.
		level.Error(b.logger).Log("msg", "batch write client failed to send profiles", "count", len(batch), "err", err)
		return err
	}

	level.Debug(b.logger).Log("msg", "batch write client has sent profiles", "count", len(batch))
	return nil
}

func isEqualLabel(a, b *profilestorepb.LabelSet) bool {
	if len(a.Labels) != len(b.Labels) {
		return false
	}

	ret := true
	for i := range a.Labels {
		if (a.Labels[i].Name != b.Labels[i].Name) || (a.Labels[i].Value != b.Labels[i].Value) {
			ret = false
		}
	}
	return ret
}

//verifies whether incoming p already is present in arr and returns end index for arr
func findIndex(arr []*profilestorepb.RawProfileSeries, p *profilestorepb.RawProfileSeries) (int, bool) {
	for i, val := range arr {
		if isEqualLabel(val.Labels, p.Labels) {
			return i, true
		}
	}
	return -1, false
}

func (b *Batcher) WriteRaw(ctx context.Context, r *profilestorepb.WriteRawRequest, opts ...grpc.CallOption) (*profilestorepb.WriteRawResponse, error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	for _, profileSeries := range r.Series {
		if j, ok := findIndex(b.series, profileSeries); ok {
			b.series[j].Samples = append(b.series[j].Samples, profileSeries.Samples...)
			continue
		}

		b.series = append(b.series, &profilestorepb.RawProfileSeries{
			Labels:  profileSeries.Labels,
			Samples: profileSeries.Samples,
		})
	}

	return &profilestorepb.WriteRawResponse{}, nil
}
