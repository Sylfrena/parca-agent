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
	"fmt"
	"reflect"
	"testing"

	profilestorepb "github.com/parca-dev/parca/gen/proto/go/parca/profilestore/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func giveSeries(series map[*profilestorepb.LabelSet][]*profilestorepb.RawSample) []*profilestorepb.RawProfileSeries {
	var profileSeries []*profilestorepb.RawProfileSeries

	for key, value := range series {
		profileSeries = append(profileSeries, &profilestorepb.RawProfileSeries{
			Labels:  key,
			Samples: value,
		})

		fmt.Printf("\n %+v \t %+v \n", key.Labels, value)
	}

	return profileSeries

}

func areEqual(a *Batcher, b *Batcher) bool {

	PrintSeries(a.series)
	fmt.Println("blah")
	p1 := giveSeries(a.series)
	p2 := giveSeries(b.series)

	var ret bool

	for i := range p1 {
		if p1[i].Labels == p2[i].Labels {
			for j := range p1[i].Samples {
				if reflect.DeepEqual((p1[i].Samples)[j].RawProfile, (p2[i].Samples)[j].RawProfile) {
					ret = true
				} else {
					ret = false
				}
			}

		} else {
			ret = false
		}
	}

	return ret
}

func TestScheduler(t *testing.T) {
	wc := NewNoopProfileStoreClient()
	batcher := NewBatcher(wc)

	labelset1 := profilestorepb.LabelSet{
		Labels: []*profilestorepb.Label{{
			Name:  "n1",
			Value: "v1",
		}},
	}

	samples1 := []*profilestorepb.RawSample{{RawProfile: []byte{44, 77, 99}}}

	fmt.Printf("\n labels thingy %+v", labelset1)
	fmt.Printf("\n rawsample thingy %+v", samples1)

	series := make(map[*profilestorepb.LabelSet][]*profilestorepb.RawSample)
	//fmt.Printf("\n rawsample thingy %+v", samples1)
	//PrintSeries(series)
	series[&labelset1] = samples1

	//fmt.Println("\n First initialisation")
	//PrintSeries(series)

	batcher.Scheduler(labelset1, samples1)

	expected_batcher1 := NewBatcher(wc)
	expected_batcher1.series = series

	assert.Equal(t, expected_batcher1.series, batcher.series)

	/*if !areEqual(expected_batcher1, batcher) {

		fmt.Println("\n BATCHER EXPECTED")
		PrintSeries(expected_batcher1.series)
		fmt.Println("BATCHER ACTUAL")
		PrintSeries(batcher.series)
		t.Errorf("ohno")

	}*/

}

/*
		labelset2 := profilestorepb.LabelSet{
		Labels: []*profilestorepb.Label{{
			Name:  "__n1__",
			Value: "value1",
		}},
	}

		samples2 := []*profilestorepb.RawSample{{RawProfile: []byte{33, 88, 88}}}



	batcher.Scheduler(labelset1, samples2)
	expected_batcher2 := &Batcher{
		series:      map[*profilestorepb.LabelSet][]*profilestorepb.RawSample{&labelset1: append(samples1, samples2...)},
		writeClient: wc,
	}

	if !reflect.DeepEqual(expected_batcher2, batcher) {

		PrintSeries(expected_batcher2.series)
		fmt.Println("BATCHER ACTUAL")
		PrintSeries(batcher.series)

		t.Errorf("ohno")
	}

	batcher.Scheduler(labelset2, samples2)
	expected_batcher3 := Batcher{
		series: map[*profilestorepb.LabelSet][]*profilestorepb.RawSample{
			&labelset1: append(samples1, samples2...),
			&labelset2: samples2,
		},
		writeClient: wc,
	}

	if !reflect.DeepEqual(expected_batcher3, batcher) {

		PrintSeries(expected_batcher2.series)
		fmt.Println("BATCHER ACTUAL")
		PrintSeries(batcher.series)
		t.Errorf("ohno")
	}
*/
