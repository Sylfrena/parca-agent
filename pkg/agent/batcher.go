package agent

import (
	"context"
	"fmt"

	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	profilestorepb "github.com/parca-dev/parca/gen/proto/go/parca/profilestore/v1alpha1"
)

type Batcher struct {
	series      map[*profilestorepb.LabelSet][]*profilestorepb.RawSample
	writeClient profilestorepb.ProfileStoreServiceClient
	logger      log.Logger

	mtx                sync.RWMutex
	lastProfileTakenAt time.Time
	lastError          error
}

//parameters: max no of requests to batch, time interval

func NewBatcher(wc profilestorepb.ProfileStoreServiceClient) *Batcher {
	return &Batcher{
		series:      make(map[*profilestorepb.LabelSet][]*profilestorepb.RawSample),
		writeClient: wc,
	}
}

func (b *Batcher) loopReport(lastProfileTakenAt time.Time, lastError error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.lastProfileTakenAt = lastProfileTakenAt
	b.lastError = lastError
}

func (b *Batcher) Run(ctx context.Context) error {

	ticker := time.NewTicker(20000000000)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		err := b.batchLoop(ctx)
		b.loopReport(time.Now(), err)
	}
}

func prettyPrint(series []*profilestorepb.RawProfileSeries) {
	for _, prof := range series {
		fmt.Printf("\n labels: %+v \n SERIES\n %+v",
			prof.Labels, prof.Samples)
	}
}

func (batcher *Batcher) batchLoop(ctx context.Context) error {

	var profseries []*profilestorepb.RawProfileSeries

	for key, value := range batcher.series {
		profseries = append(profseries, &profilestorepb.RawProfileSeries{
			Labels:  key,
			Samples: value,
		})

	}

	fmt.Println("this is what the client is writing")
	prettyPrint(profseries)

	_, err := batcher.writeClient.WriteRaw(ctx,
		&profilestorepb.WriteRawRequest{Series: profseries})

	if err != nil {
		level.Error(batcher.logger).Log("msg", "writeclient failed to send profiles", "err", err)
		return err
	}

	return nil
}

// map labelSet, sample
func (batcher *Batcher) Scheduler(labelset profilestorepb.LabelSet, samples []*profilestorepb.RawSample) Batcher {
	//agg(requests) -> batch
	batcher.mtx.Lock()
	defer batcher.mtx.Unlock()

	batcher.series[&labelset] = samples

	//	fmt.Println("\n batcher series from scheduler be %+v", batcher.series)

	return *batcher
}

//don't give channel access to implementation details
