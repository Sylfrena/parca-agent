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

package discovery

import (
	"context"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	profilestorepb "github.com/parca-dev/parca/gen/proto/go/parca/profilestore/v1alpha1"
	"github.com/prometheus/common/model"

	"github.com/parca-dev/parca-agent/pkg/agent"
	"github.com/parca-dev/parca-agent/pkg/debuginfo"
	"github.com/parca-dev/parca-agent/pkg/ksym"
)

type Profiler interface {
	Labels() model.LabelSet
	LastProfileTakenAt() time.Time
	LastError() error
}
type TargetManager struct {
	mtx               *sync.RWMutex
	targetSets        map[string][]*Group
	activeProfilers   map[string][]Profiler
	logger            log.Logger
	externalLabels    map[string]string
	ksymCache         *ksym.KsymCache
	writeClient       profilestorepb.ProfileStoreServiceClient
	debugInfoClient   debuginfo.Client
	sink              func(agent.Record)
	profilingDuration time.Duration
	tmp               string
}

func NewTargetManager(
	logger log.Logger,
	externalLabels map[string]string,
	ksymCache *ksym.KsymCache,
	writeClient profilestorepb.ProfileStoreServiceClient,
	debugInfoClient debuginfo.Client,
	profilingDuration time.Duration,
	tmp string,
) *TargetManager {
	m := &TargetManager{
		mtx:               &sync.RWMutex{},
		targetSets:        map[string][]*Group{},
		activeProfilers:   map[string][]Profiler{},
		logger:            logger,
		externalLabels:    externalLabels,
		ksymCache:         ksymCache,
		writeClient:       writeClient,
		debugInfoClient:   debugInfoClient,
		profilingDuration: profilingDuration,
		tmp:               tmp,
	}

	return m
}

func (m *TargetManager) Run(ctx context.Context, update <-chan map[string][]*Group) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case targetSets := <-update:
			err := m.reconcileTargets(ctx, targetSets)
			if err != nil {
				return err
			}
		}
	}
}

func (m *TargetManager) reconcileTargets(ctx context.Context, targetSets map[string][]*Group) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	level.Debug(m.logger).Log("msg", "reconciling targets")

	for name, targetSet := range targetSets {
		level.Debug(m.logger).Log("msg", "reconciling target groups", "name", name)

		if _, found := m.targetSets[name]; !found {
			m.targetSets[name] = targetSet
			profilerSet := []Profiler{}

			for _, targetGroup := range targetSet {
				level.Debug(m.logger).Log("msg", "iterating through targetgroups")
				level.Debug(m.logger).Log("msg", "targetgroup: ", "labels ", targetGroup.Labels.String())
				level.Debug(m.logger).Log("msg", "targetgroup: ", "source ", targetGroup.Source)

				for _, target := range targetGroup.Targets {

					level.Debug(m.logger).Log("msg", "iterating through targets ", "target: ", target.String())

					for k, v := range m.externalLabels {
						target[model.LabelName(k)] = model.LabelValue(v)
					}

					newProfiler := agent.NewCgroupProfiler(
						m.logger,
						m.ksymCache,
						m.writeClient,
						m.debugInfoClient,
						target,
						m.profilingDuration,
						m.tmp,
					)

					go newProfiler.Run(ctx)

					profilerSet = append(profilerSet, newProfiler)
				}

			}

			m.activeProfilers[name] = profilerSet

		}

	}
	return nil
}

func (m *TargetManager) ActiveProfilers() []Profiler {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	profilerSet := []Profiler{}
	for _, profilers := range m.activeProfilers {

		profilerSet = append(profilerSet, profilers...)

	}

	return profilerSet

}
