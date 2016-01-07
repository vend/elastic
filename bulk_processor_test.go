// Copyright 2012-2016 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"
)

func TestBulkProcessorDefaults(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	p := NewBulkProcessor(client)
	if p == nil {
		t.Fatalf("expected BulkProcessor; got: %v", p)
	}
	if got, want := p.running, false; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
	if got, want := p.name, ""; got != want {
		t.Errorf("expected %q; got: %q", want, got)
	}
	if got, want := p.numWorkers, 1; got != want {
		t.Errorf("expected %d; got: %d", want, got)
	}
	if got, want := p.bulkActions, 1000; got != want {
		t.Errorf("expected %d; got: %d", want, got)
	}
	if got, want := p.bulkByteSize, 5*1024*1024; got != want {
		t.Errorf("expected %d; got: %d", want, got)
	}
	if got, want := p.flushInterval, time.Duration(0); got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
	if got, want := p.wantStats, false; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
}

func TestBulkProcessorBasedOnBulkActions(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndLog(t, SetTraceLog(log.New(os.Stdout, "", 0)))
	client := setupTestClientAndCreateIndex(t)

	testBulkProcessor(t,
		10000,
		NewBulkProcessor(client).
			Name("ByteActions-1").
			Workers(1).
			BulkActions(100).
			BulkByteSize(-1),
	)

	testBulkProcessor(t,
		10000,
		NewBulkProcessor(client).
			Name("ByteActions-2").
			Workers(2).
			BulkActions(100).
			BulkByteSize(-1),
	)
}

func TestBulkProcessorBasedOnBulkSizeInBytes(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndLog(t, SetTraceLog(log.New(os.Stdout, "", 0)))
	client := setupTestClientAndCreateIndex(t)

	testBulkProcessor(t,
		10000,
		NewBulkProcessor(client).
			Name("ByteSize-1").
			Workers(1).
			BulkActions(-1).
			BulkByteSize(64*1024),
	)

	testBulkProcessor(t,
		10000,
		NewBulkProcessor(client).
			Name("ByteSize-2").
			Workers(2).
			BulkActions(-1).
			BulkByteSize(64*1024),
	)
}

func TestBulkProcessorBasedOnFlushInterval(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndLog(t, SetTraceLog(log.New(os.Stdout, "", 0)))
	client := setupTestClientAndCreateIndex(t)

	var beforeRequests int64
	var befores int64
	var afters int64
	var failures int64

	beforeCallback := func(executionId int64, requests []BulkableRequest) {
		atomic.AddInt64(&beforeRequests, int64(len(requests)))
		atomic.AddInt64(&befores, 1)
	}
	afterCallback := func(executionId int64, response *BulkResponse) {
		atomic.AddInt64(&afters, 1)
	}
	failureCallback := func(executionId int64, response *BulkResponse, err error) {
		atomic.AddInt64(&failures, 1)
	}

	p := NewBulkProcessor(client).
		Name("FlushInterval-1").
		Workers(2).
		BulkActions(-1).
		BulkByteSize(-1).
		FlushInterval(1 * time.Second).
		Before(beforeCallback).After(afterCallback).Failure(failureCallback)

	err := p.Do()
	if err != nil {
		t.Fatal(err)
	}

	const numDocs = 1000 // low-enough number that flush should be invoked

	for i := 1; i <= numDocs; i++ {
		tweet := tweet{User: "olivere", Message: fmt.Sprintf("%d. %s", i, randomString(rand.Intn(64)))}
		request := NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id(fmt.Sprintf("%d", i)).Doc(tweet)
		p.Add(request)
	}

	// Should flush at least once
	time.Sleep(2 * time.Second)

	err = p.Close()
	if err != nil {
		t.Fatal(err)
	}

	if p.stats.Flushed == 0 {
		t.Errorf("expected at least 1 flush; got: %d", p.stats.Flushed)
	}
	if got, want := beforeRequests, int64(numDocs); got != want {
		t.Errorf("expected %d requests to before callback; got: %d", want, got)
	}
	if befores == 0 {
		t.Error("expected at least 1 call to before callback")
	}
	if afters == 0 {
		t.Error("expected at least 1 call to after callback")
	}
	if failures != 0 {
		t.Errorf("expected 0 calls to failure callback; got: %d", failures)
	}

	// Check number of documents that were bulk indexed
	_, err = p.c.Flush(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	count, err := p.c.Count(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if count != int64(numDocs) {
		t.Fatalf("expected %d documents; got: %d", numDocs, count)
	}
}

func TestBulkProcessorClose(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndLog(t, SetTraceLog(log.New(os.Stdout, "", 0)))
	client := setupTestClientAndCreateIndex(t)

	var beforeRequests int64
	var befores int64
	var afters int64
	var failures int64

	beforeCallback := func(executionId int64, requests []BulkableRequest) {
		atomic.AddInt64(&beforeRequests, int64(len(requests)))
		atomic.AddInt64(&befores, 1)
	}
	afterCallback := func(executionId int64, response *BulkResponse) {
		atomic.AddInt64(&afters, 1)
	}
	failureCallback := func(executionId int64, response *BulkResponse, err error) {
		atomic.AddInt64(&failures, 1)
	}

	p := NewBulkProcessor(client).
		Name("FlushInterval-1").
		Workers(2).
		BulkActions(-1).
		BulkByteSize(-1).
		FlushInterval(30 * time.Second). // 30 seconds to flush
		Before(beforeCallback).After(afterCallback).Failure(failureCallback)

	err := p.Do()
	if err != nil {
		t.Fatal(err)
	}

	const numDocs = 1000 // low-enough number that flush should be invoked

	for i := 1; i <= numDocs; i++ {
		tweet := tweet{User: "olivere", Message: fmt.Sprintf("%d. %s", i, randomString(rand.Intn(64)))}
		request := NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id(fmt.Sprintf("%d", i)).Doc(tweet)
		p.Add(request)
	}

	// Should not flush because 30s > 1s
	time.Sleep(1 * time.Second)

	// Close should flush
	err = p.Close()
	if err != nil {
		t.Fatal(err)
	}

	if p.stats.Flushed != 0 {
		t.Errorf("expected no flush; got: %d", p.stats.Flushed)
	}
	if got, want := beforeRequests, int64(numDocs); got != want {
		t.Errorf("expected %d requests to before callback; got: %d", want, got)
	}
	if befores == 0 {
		t.Error("expected at least 1 call to before callback")
	}
	if afters == 0 {
		t.Error("expected at least 1 call to after callback")
	}
	if failures != 0 {
		t.Errorf("expected 0 calls to failure callback; got: %d", failures)
	}

	// Check number of documents that were bulk indexed
	_, err = p.c.Flush(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	count, err := p.c.Count(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if count != int64(numDocs) {
		t.Fatalf("expected %d documents; got: %d", numDocs, count)
	}
}

func TestBulkProcessorFlush(t *testing.T) {
	//client := setupTestClientAndCreateIndexAndLog(t, SetTraceLog(log.New(os.Stdout, "", 0)))
	client := setupTestClientAndCreateIndex(t)

	p := NewBulkProcessor(client).
		Name("ManualFlush").
		Workers(10).
		BulkActions(-1).
		BulkByteSize(-1).
		FlushInterval(30 * time.Second). // 30 seconds to flush
		CollectStats(true)

	err := p.Do()
	if err != nil {
		t.Fatal(err)
	}

	const numDocs = 100

	for i := 1; i <= numDocs; i++ {
		tweet := tweet{User: "olivere", Message: fmt.Sprintf("%d. %s", i, randomString(rand.Intn(64)))}
		request := NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id(fmt.Sprintf("%d", i)).Doc(tweet)
		p.Add(request)
	}

	// Should not flush because 30s > 1s
	time.Sleep(1 * time.Second)

	// No flush yet
	stats := p.Stats()
	if stats.Flushed != 0 {
		t.Errorf("expected no flush; got: %d", p.stats.Flushed)
	}

	// Manual flush
	err = p.Flush()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	// Now flushed
	stats = p.Stats()
	if got, want := p.stats.Flushed, int64(1); got != want {
		t.Errorf("expected %d flush; got: %d", want, got)
	}

	// Close should not start another flush
	err = p.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Still 1 flush
	stats = p.Stats()
	if got, want := p.stats.Flushed, int64(1); got != want {
		t.Errorf("expected %d flush; got: %d", want, got)
	}

	// Check number of documents that were bulk indexed
	_, err = p.c.Flush(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	count, err := p.c.Count(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if count != int64(numDocs) {
		t.Fatalf("expected %d documents; got: %d", numDocs, count)
	}
}

// -- Helper --

func testBulkProcessor(t *testing.T, numDocs int, p *BulkProcessor) {
	var beforeRequests int64
	var befores int64
	var afters int64
	var failures int64

	beforeCallback := func(executionId int64, requests []BulkableRequest) {
		atomic.AddInt64(&beforeRequests, int64(len(requests)))
		atomic.AddInt64(&befores, 1)
	}
	afterCallback := func(executionId int64, response *BulkResponse) {
		atomic.AddInt64(&afters, 1)
	}
	failureCallback := func(executionId int64, response *BulkResponse, err error) {
		atomic.AddInt64(&failures, 1)
	}

	err := p.CollectStats(true).
		Before(beforeCallback).
		After(afterCallback).
		Failure(failureCallback).
		Do()
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= numDocs; i++ {
		tweet := tweet{User: "olivere", Message: fmt.Sprintf("%d. %s", i, randomString(rand.Intn(64)))}
		request := NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id(fmt.Sprintf("%d", i)).Doc(tweet)
		p.Add(request)

		_ = p.Stats()
	}

	err = p.Close()
	if err != nil {
		t.Fatal(err)
	}

	stats := p.Stats()

	if stats.Flushed != 0 {
		t.Errorf("expected no flush; got: %d", stats.Flushed)
	}
	if stats.Committed <= 0 {
		t.Errorf("expected committed > %d; got: %d", 0, stats.Committed)
	}
	if got, want := stats.Indexed, int64(numDocs); got != want {
		t.Errorf("expected indexed = %d; got: %d", want, got)
	}
	if got, want := stats.Created, int64(0); got != want {
		t.Errorf("expected created = %d; got: %d", want, got)
	}
	if got, want := stats.Updated, int64(0); got != want {
		t.Errorf("expected updated = %d; got: %d", want, got)
	}
	if got, want := stats.Deleted, int64(0); got != want {
		t.Errorf("expected deleted = %d; got: %d", want, got)
	}
	if got, want := stats.Succeeded, int64(numDocs); got != want {
		t.Errorf("expected succeeded = %d; got: %d", want, got)
	}
	if got, want := stats.Failed, int64(0); got != want {
		t.Errorf("expected failed = %d; got: %d", want, got)
	}
	if got, want := beforeRequests, int64(numDocs); got != want {
		t.Errorf("expected %d requests to before callback; got: %d", want, got)
	}
	if befores == 0 {
		t.Error("expected at least 1 call to before callback")
	}
	if afters == 0 {
		t.Error("expected at least 1 call to after callback")
	}
	if failures != 0 {
		t.Errorf("expected 0 calls to failure callback; got: %d", failures)
	}

	// Check number of documents that were bulk indexed
	_, err = p.c.Flush(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	count, err := p.c.Count(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if count != int64(numDocs) {
		t.Fatalf("expected %d documents; got: %d", numDocs, count)
	}
}
