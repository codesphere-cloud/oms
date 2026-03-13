// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import "time"

type Time interface {
	Sleep(time.Duration)
	Now() time.Time
}

type RealTime struct{}

func NewTime() *RealTime {
	return &RealTime{}
}

func (r *RealTime) Now() time.Time {
	return time.Now()
}

func (r *RealTime) Sleep(t time.Duration) {
	time.Sleep(t)
}

func NewFakeTime() *FakeTime {
	return &FakeTime{
		CurrentTime: time.Date(2026, 01, 01, 0, 0, 0, 0, time.UTC),
	}
}

type FakeTime struct {
	CurrentTime time.Time
}

func (f *FakeTime) Now() time.Time {
	return f.CurrentTime
}

func (f *FakeTime) Sleep(t time.Duration) {
	f.CurrentTime = f.CurrentTime.Add(t)
}
