package agent

import (
	"context"
	"fmt"

	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	profilestorepb "github.com/parca-dev/parca/gen/proto/go/parca/profilestore/v1alpha1"
)

type Batcher struct {
	series map[uint64]*profilestorepb.RawProfileSeries
	//writeClient profilestorepb.ProfileStoreServiceClient
	logger log.Logger

	mtx                *sync.RWMutex
	lastProfileTakenAt time.Time
	lastError          error
}

func NewBatcher() *Batcher {
	return &Batcher{
		series: make(map[uint64]*profilestorepb.RawProfileSeries),
		//writeClient: wc,
		mtx: &sync.RWMutex{},
	}
}

func (b *Batcher) loopReport(lastProfileTakenAt time.Time, lastError error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.lastProfileTakenAt = lastProfileTakenAt
	b.lastError = lastError
}

func (b *Batcher) Run(ctx context.Context) error {
	// TODO(Sylfrena): Make ticker duration configurable
	const tickerDuration = 10 * time.Second

	ticker := time.NewTicker(tickerDuration)
	defer ticker.Stop()

	var err error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		err := b.batchLoop(ctx)
		b.series = make(map[uint64]*profilestorepb.RawProfileSeries)

		b.loopReport(time.Now(), err)
	}
	return err
}

func (b *Batcher) batchLoop(ctx context.Context) error {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	var profileSeries []*profilestorepb.RawProfileSeries

	for _, value := range b.series {
		profileSeries = append(profileSeries, &profilestorepb.RawProfileSeries{
			Labels:  value.Labels,
			Samples: value.Samples,
		})

	}

	fmt.Printf("sending it %+v", profileSeries)

	_, err := b.WriteRaw(ctx,
		&profilestorepb.WriteRawRequest{Series: profileSeries})

	if err != nil {
		level.Error(b.logger).Log("msg", "Writeclient failed to send profiles", "err", err)
		return err
	}

	return nil
}

func (b *Batcher) Scheduler(profileSeries *profilestorepb.RawProfileSeries) {

}

func hash(ls profilestorepb.LabelSet) uint64 {
	var seps = []byte{'\xff'}
	b := make([]byte, 0, 1024)
	for _, v := range ls.Labels {
		if len(b)+len(v.Name)+len(v.Value)+2 >= cap(b) {
			// If labels entry is 1KB+ do not allocate whole entry.
			h := xxhash.New()
			_, _ = h.Write(b)
			_, _ = h.WriteString(v.Name)
			_, _ = h.Write(seps)
			_, _ = h.WriteString(v.Value)
			_, _ = h.Write(seps)
			return h.Sum64()
		}

		b = append(b, v.Name...)
		b = append(b, seps[0])
		b = append(b, v.Value...)
		b = append(b, seps[0])
	}
	return xxhash.Sum64(b)
}

func (b *Batcher) WriteRaw(ctx context.Context, r *profilestorepb.WriteRawRequest) (*profilestorepb.WriteRawResponse, error) {

	b.mtx.Lock()
	defer b.mtx.Unlock()

	for _, profileSeries := range r.Series {
		labelsetHash := hash(*profileSeries.Labels)

		existing_sample, ok := b.series[labelsetHash]
		if ok {
			b.series[labelsetHash].Samples = append(existing_sample.Samples, profileSeries.Samples...)
		} else {
			b.series[labelsetHash] = &profilestorepb.RawProfileSeries{}
			b.series[labelsetHash].Samples = profileSeries.Samples
		}

	}
	return &profilestorepb.WriteRawResponse{}, nil

}
