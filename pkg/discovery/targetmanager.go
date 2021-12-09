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
	"github.com/prometheus/prometheus/pkg/labels"

	"github.com/parca-dev/parca-agent/pkg/agent"
	"github.com/parca-dev/parca-agent/pkg/debuginfo"
	"github.com/parca-dev/parca-agent/pkg/ksym"
)

type Profiler interface {
	Labels() model.LabelSet
	LastProfileTakenAt() time.Time
	LastError() error
	Stop()
}

type ProfilerPool struct {
	ctx               context.Context
	mtx               *sync.Mutex
	activeTargets     map[uint64]*Target
	activeProfilers   map[uint64]Profiler
	externalLabels    model.LabelSet
	logger            log.Logger
	ksymCache         *ksym.KsymCache
	writeClient       profilestorepb.ProfileStoreServiceClient
	debugInfoClient   debuginfo.Client
	profilingDuration time.Duration
	tmp               string
}

func NewProfilerPool(
	ctx context.Context,
	externalLabels model.LabelSet,
	logger log.Logger,
	ksymCache *ksym.KsymCache,
	writeClient profilestorepb.ProfileStoreServiceClient,
	debugInfoClient debuginfo.Client,
	profilingDuration time.Duration,
	tmp string) *ProfilerPool {
	pp := &ProfilerPool{
		ctx:               ctx,
		mtx:               &sync.Mutex{},
		activeTargets:     map[uint64]*Target{},
		activeProfilers:   map[uint64]Profiler{},
		externalLabels:    externalLabels,
		logger:            logger,
		ksymCache:         ksymCache,
		writeClient:       writeClient,
		debugInfoClient:   debugInfoClient,
		profilingDuration: profilingDuration,
		tmp:               tmp,
	}

	return pp
}

func (pp *ProfilerPool) Profilers() []Profiler {
	pp.mtx.Lock()
	defer pp.mtx.Unlock()

	res := make([]Profiler, 0, len(pp.activeProfilers))
	for _, profiler := range pp.activeProfilers {
		res = append(res, profiler)
	}
	return res
}

func (pp *ProfilerPool) Sync(tg []*Group) {
	pp.mtx.Lock()
	defer pp.mtx.Unlock()

	newTargets := map[uint64]*Target{}

	for _, newTargetGroup := range tg {
		for _, t := range newTargetGroup.Targets {

			target := &Target{labelSet: model.LabelSet{}}

			for labelName, labelValue := range t {
				target.labelSet[labelName] = labelValue
			}

			for labelName, labelValue := range newTargetGroup.Labels {
				target.labelSet[labelName] = labelValue
			}

			for labelName, labelValue := range pp.externalLabels {
				target.labelSet[labelName] = labelValue
			}

			h := labelsetToLabels(target.labelSet).Hash()
			newTargets[h] = target
		}
	}

	//add new targets and profile them
	for _, newTarget := range newTargets {
		h := labelsetToLabels(newTarget.labelSet).Hash()

		if _, found := pp.activeTargets[h]; !found {

			newProfiler := agent.NewCgroupProfiler(
				pp.logger,
				pp.ksymCache,
				pp.writeClient,
				pp.debugInfoClient,
				newTarget.labelSet,
				pp.profilingDuration,
				pp.tmp,
			)

			go func() {
				err := newProfiler.Run(pp.ctx)
				level.Debug(pp.logger).Log("msg", "profiler ended with error", "error", err, "labels", newProfiler.Labels().String())
			}()

			pp.activeTargets[h] = newTarget
			pp.activeProfilers[h] = newProfiler
		}
	}

	// delete profiles no longer active
	for h := range pp.activeTargets {
		if _, found := newTargets[h]; !found {

			pp.activeProfilers[h].Stop()

			delete(pp.activeTargets, h)
			delete(pp.activeProfilers, h)
		}
	}
}

/*func containsTarget(t *Target, targets []*Target) bool {
	for _, target := range targets {
		if t.labelSet.Equal(target.labelSet) {
			return true
		}
	}
	return false
}*/

func labelsetToLabels(labelSet model.LabelSet) labels.Labels {
	ls := make(labels.Labels, 0, len(labelSet))
	for k, v := range labelSet {
		ls = append(ls, labels.Label{
			Name:  string(k),
			Value: string(v),
		})
	}
	return ls
}

func toStringMap(labelSet model.LabelSet) map[string]string {
	m := map[string]string{}

	for k, v := range labelSet {
		m[string(k)] = string(v)
	}

	return m
}

type Target struct {
	labelSet model.LabelSet
}
type TargetManager struct {
	mtx               *sync.RWMutex
	profilerPools     map[string]*ProfilerPool
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
	tmp string) *TargetManager {
	m := &TargetManager{
		mtx:               &sync.RWMutex{},
		profilerPools:     map[string]*ProfilerPool{},
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
		pp, found := m.profilerPools[name]

		if !found {
			pp = NewProfilerPool(ctx, model.LabelSet{}, m.logger, m.ksymCache, m.writeClient, m.debugInfoClient, m.profilingDuration, m.tmp)

			/*m.targetSets[name] = targetSet
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

					go func() {
						err := newProfiler.Run(ctx)
						level.Debug(m.logger).Log("msg", "profiler ended with error", "error", err, "labels", newProfiler.Labels().String())
					}()

					profilerSet = append(profilerSet, newProfiler)
				}

			}

			m.activeProfilers[name] = profilerSet
			*/
		}

		pp.Sync(targetSet)
	}
	return nil
}

func (m *TargetManager) ActiveProfilers() map[string][]Profiler {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	profilerSet := map[string][]Profiler{}
	for name, profilerPool := range m.profilerPools {

		profilerSet[name] = profilerPool.Profilers()
	}

	return profilerSet
}
