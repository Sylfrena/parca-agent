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

	"github.com/google/pprof/profile"
	profilestorepb "github.com/parca-dev/parca/gen/proto/go/parca/profilestore/v1alpha1"
	"github.com/prometheus/tsdb/labels"
	"google.golang.org/grpc"
)

type ProfileListener struct {
	next      profilestorepb.ProfileStoreServiceClient
	observers []*observer
	omtx      *sync.Mutex
}

func NewProfileListener(next profilestorepb.ProfileStoreServiceClient) *ProfileListener {
	return &ProfileListener{next: next}
}

func (l *ProfileListener) WriteRaw(ctx context.Context, r *profilestorepb.WriteRawRequest, opts ...grpc.CallOption) (*profilestorepb.WriteRawResponse, error) {
	return l.next.WriteRaw(ctx, r, opts...)
}

type observer struct {
	f func(*profilestorepb.WriteRawRequest)
}

func (l *ProfileListener) Observe(f func(*profilestorepb.WriteRawRequest)) *observer {
	l.omtx.Lock()
	defer l.omtx.Unlock()

	o := &observer{
		f: f,
	}
	l.observers = append(l.observers, o)
	return o
}

func (l *ProfileListener) RemoveObserver(o *observer) {
	l.omtx.Lock()
	defer l.omtx.Unlock()

	found := false
	i := 0
	for ; i < len(l.observers); i++ {
		if l.observers[i] == o {
			found = true
			break
		}
	}
	if found {
		l.observers = append(l.observers[:i], l.observers[i+1:]...)
	}
}

func (l *ProfileListener) NextMatchingProfile(ctx context.Context, matchers []*labels.Matcher) (*profile.Profile, error) {
<<<<<<< HEAD
	pCh := make(chan []byte)
=======
	pCh := make(chan *profile.Profile)
>>>>>>> a197feb (add listener, doesn't compile yet)
	defer close(pCh)

	o := l.Observe(func(r *profilestorepb.WriteRawRequest) {
		profileLabels := map[string]string{}

		//for _, series := range

		for _, label := range r.Labels {
			profileLabels[label.Name] = label.Value
		}

		for _, matcher := range matchers {
			labelValue := profileLabels[matcher.Name]
			if !matcher.Matches(labelValue) {
				return
			}
		}

		pCh <- r.Profile.Copy()
	})
	defer l.RemoveObserver(o)

	select {
	case p := <-pCh:
		return p, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
