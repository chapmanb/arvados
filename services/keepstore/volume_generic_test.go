package main

import (
	"bytes"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// A TestableVolumeFactory returns a new TestableVolume. The factory
// function, and the TestableVolume it returns, can use "t" to write
// logs, fail the current test, etc.
type TestableVolumeFactory func(t *testing.T) TestableVolume

// DoGenericVolumeTests runs a set of tests that every TestableVolume
// is expected to pass. It calls factory to create a new TestableVolume
// for each test case, to avoid leaking state between tests.
func DoGenericVolumeTests(t *testing.T, factory TestableVolumeFactory) {
	testGet(t, factory)
	testGetNoSuchBlock(t, factory)

	testCompareSameContent(t, factory)
	testCompareWithDifferentContent(t, factory)
	testCompareWithBadData(t, factory)

	testPutBlockWithSameContent(t, factory)
	testPutBlockWithDifferentContent(t, factory)
	testPutMultipleBlocks(t, factory)

	testPutAndTouch(t, factory)
	testTouchNoSuchBlock(t, factory)

	testMtimeNoSuchBlock(t, factory)

	testIndexTo(t, factory)

	testDeleteNewBlock(t, factory)
	testDeleteOldBlock(t, factory)
	testDeleteNoSuchBlock(t, factory)

	testStatus(t, factory)

	testString(t, factory)

	testUpdateReadOnly(t, factory)

	testGetConcurrent(t, factory)
	testPutConcurrent(t, factory)
}

// Put a test block, get it and verify content
// Test should pass for both writable and read-only volumes
func testGet(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	v.PutRaw(TestHash, TestBlock)

	buf, err := v.Get(TestHash)
	if err != nil {
		t.Error(err)
	}

	bufs.Put(buf)

	if bytes.Compare(buf, TestBlock) != 0 {
		t.Errorf("expected %s, got %s", string(TestBlock), string(buf))
	}
}

// Invoke get on a block that does not exist in volume; should result in error
// Test should pass for both writable and read-only volumes
func testGetNoSuchBlock(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if _, err := v.Get(TestHash2); err == nil {
		t.Errorf("Expected error while getting non-existing block %v", TestHash2)
	}
}

// Put a test block and compare the locator with same content
// Test should pass for both writable and read-only volumes
func testCompareSameContent(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	v.PutRaw(TestHash, TestBlock)

	// Compare the block locator with same content
	err := v.Compare(TestHash, TestBlock)
	if err != nil {
		t.Errorf("Got err %q, expected nil", err)
	}
}

// Put a test block and compare the locator with a different content
// Expect error due to collision
// Test should pass for both writable and read-only volumes
func testCompareWithDifferentContent(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	v.PutRaw(TestHash, TestBlock)

	// Compare the block locator with different content; collision
	err := v.Compare(TestHash, []byte("baddata"))
	if err == nil {
		t.Errorf("Expected error due to collision")
	}
}

// Put a test block with bad data (hash does not match, but Put does not verify)
// Compare the locator with good data whose hash matches with locator
// Expect error due to corruption.
// Test should pass for both writable and read-only volumes
func testCompareWithBadData(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	v.PutRaw(TestHash, []byte("baddata"))

	err := v.Compare(TestHash, TestBlock)
	if err == nil {
		t.Errorf("Expected error due to corruption")
	}
}

// Put a block and put again with same content
// Test is intended for only writable volumes
func testPutBlockWithSameContent(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if v.Writable() == false {
		return
	}

	err := v.Put(TestHash, TestBlock)
	if err != nil {
		t.Errorf("Got err putting block %q: %q, expected nil", TestBlock, err)
	}

	err = v.Put(TestHash, TestBlock)
	if err != nil {
		t.Errorf("Got err putting block second time %q: %q, expected nil", TestBlock, err)
	}
}

// Put a block and put again with different content
// Test is intended for only writable volumes
func testPutBlockWithDifferentContent(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if v.Writable() == false {
		return
	}

	err := v.Put(TestHash, TestBlock)
	if err != nil {
		t.Errorf("Got err putting block %q: %q, expected nil", TestBlock, err)
	}

	putErr := v.Put(TestHash, TestBlock2)
	buf, getErr := v.Get(TestHash)
	if putErr == nil {
		// Put must not return a nil error unless it has
		// overwritten the existing data.
		if bytes.Compare(buf, TestBlock2) != 0 {
			t.Errorf("Put succeeded but Get returned %+v, expected %+v", buf, TestBlock2)
		}
	} else {
		// It is permissible for Put to fail, but it must
		// leave us with either the original data, the new
		// data, or nothing at all.
		if getErr == nil && bytes.Compare(buf, TestBlock) != 0 && bytes.Compare(buf, TestBlock2) != 0 {
			t.Errorf("Put failed but Get returned %+v, which is neither %+v nor %+v", buf, TestBlock, TestBlock2)
		}
	}
	if getErr == nil {
		bufs.Put(buf)
	}
}

// Put and get multiple blocks
// Test is intended for only writable volumes
func testPutMultipleBlocks(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if v.Writable() == false {
		return
	}

	err := v.Put(TestHash, TestBlock)
	if err != nil {
		t.Errorf("Got err putting block %q: %q, expected nil", TestBlock, err)
	}

	err = v.Put(TestHash2, TestBlock2)
	if err != nil {
		t.Errorf("Got err putting block %q: %q, expected nil", TestBlock2, err)
	}

	err = v.Put(TestHash3, TestBlock3)
	if err != nil {
		t.Errorf("Got err putting block %q: %q, expected nil", TestBlock3, err)
	}

	data, err := v.Get(TestHash)
	if err != nil {
		t.Error(err)
	} else if bytes.Compare(data, TestBlock) != 0 {
		t.Errorf("Block present, but content is incorrect: Expected: %v  Found: %v", data, TestBlock)
	}
	bufs.Put(data)

	data, err = v.Get(TestHash2)
	if err != nil {
		t.Error(err)
	} else if bytes.Compare(data, TestBlock2) != 0 {
		t.Errorf("Block present, but content is incorrect: Expected: %v  Found: %v", data, TestBlock2)
	}
	bufs.Put(data)

	data, err = v.Get(TestHash3)
	if err != nil {
		t.Error(err)
	} else if bytes.Compare(data, TestBlock3) != 0 {
		t.Errorf("Block present, but content is incorrect: Expected: %v  Found: %v", data, TestBlock3)
	}
	bufs.Put(data)
}

// testPutAndTouch
//   Test that when applying PUT to a block that already exists,
//   the block's modification time is updated.
// Test is intended for only writable volumes
func testPutAndTouch(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if v.Writable() == false {
		return
	}

	if err := v.Put(TestHash, TestBlock); err != nil {
		t.Error(err)
	}

	// We'll verify { t0 < threshold < t1 }, where t0 is the
	// existing block's timestamp on disk before Put() and t1 is
	// its timestamp after Put().
	threshold := time.Now().Add(-time.Second)

	// Set the stored block's mtime far enough in the past that we
	// can see the difference between "timestamp didn't change"
	// and "timestamp granularity is too low".
	v.TouchWithDate(TestHash, time.Now().Add(-20*time.Second))

	// Make sure v.Mtime() agrees the above Utime really worked.
	if t0, err := v.Mtime(TestHash); err != nil || t0.IsZero() || !t0.Before(threshold) {
		t.Errorf("Setting mtime failed: %v, %v", t0, err)
	}

	// Write the same block again.
	if err := v.Put(TestHash, TestBlock); err != nil {
		t.Error(err)
	}

	// Verify threshold < t1
	if t1, err := v.Mtime(TestHash); err != nil {
		t.Error(err)
	} else if t1.Before(threshold) {
		t.Errorf("t1 %v should be >= threshold %v after v.Put ", t1, threshold)
	}
}

// Touching a non-existing block should result in error.
// Test should pass for both writable and read-only volumes
func testTouchNoSuchBlock(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if err := v.Touch(TestHash); err == nil {
		t.Error("Expected error when attempted to touch a non-existing block")
	}
}

// Invoking Mtime on a non-existing block should result in error.
// Test should pass for both writable and read-only volumes
func testMtimeNoSuchBlock(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if _, err := v.Mtime("12345678901234567890123456789012"); err == nil {
		t.Error("Expected error when updating Mtime on a non-existing block")
	}
}

// Put a few blocks and invoke IndexTo with:
// * no prefix
// * with a prefix
// * with no such prefix
// Test should pass for both writable and read-only volumes
func testIndexTo(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	v.PutRaw(TestHash, TestBlock)
	v.PutRaw(TestHash2, TestBlock2)
	v.PutRaw(TestHash3, TestBlock3)

	buf := new(bytes.Buffer)
	v.IndexTo("", buf)
	indexRows := strings.Split(string(buf.Bytes()), "\n")
	sort.Strings(indexRows)
	sortedIndex := strings.Join(indexRows, "\n")
	m, err := regexp.MatchString(
		`^\n`+TestHash+`\+\d+ \d+\n`+
			TestHash3+`\+\d+ \d+\n`+
			TestHash2+`\+\d+ \d+$`,
		sortedIndex)
	if err != nil {
		t.Error(err)
	} else if !m {
		t.Errorf("Got index %q for empty prefix", sortedIndex)
	}

	for _, prefix := range []string{"f", "f15", "f15ac"} {
		buf = new(bytes.Buffer)
		v.IndexTo(prefix, buf)

		m, err := regexp.MatchString(`^`+TestHash2+`\+\d+ \d+\n$`, string(buf.Bytes()))
		if err != nil {
			t.Error(err)
		} else if !m {
			t.Errorf("Got index %q for prefix %s", string(buf.Bytes()), prefix)
		}
	}

	for _, prefix := range []string{"zero", "zip", "zilch"} {
		buf = new(bytes.Buffer)
		v.IndexTo(prefix, buf)
		if err != nil {
			t.Errorf("Got error on IndexTo with no such prefix %v", err.Error())
		} else if buf.Len() != 0 {
			t.Errorf("Expected empty list for IndexTo with no such prefix %s", prefix)
		}
	}
}

// Calling Delete() for a block immediately after writing it (not old enough)
// should neither delete the data nor return an error.
// Test is intended for only writable volumes
func testDeleteNewBlock(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if v.Writable() == false {
		return
	}

	v.Put(TestHash, TestBlock)

	if err := v.Delete(TestHash); err != nil {
		t.Error(err)
	}
	data, err := v.Get(TestHash)
	if err != nil {
		t.Error(err)
	} else if bytes.Compare(data, TestBlock) != 0 {
		t.Error("Block still present, but content is incorrect: %+v != %+v", data, TestBlock)
	}
	bufs.Put(data)
}

// Calling Delete() for a block with a timestamp older than
// blob_signature_ttl seconds in the past should delete the data.
// Test is intended for only writable volumes
func testDeleteOldBlock(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if v.Writable() == false {
		return
	}

	v.Put(TestHash, TestBlock)
	v.TouchWithDate(TestHash, time.Now().Add(-2*blob_signature_ttl*time.Second))

	if err := v.Delete(TestHash); err != nil {
		t.Error(err)
	}
	if _, err := v.Get(TestHash); err == nil || !os.IsNotExist(err) {
		t.Errorf("os.IsNotExist(%v) should have been true", err.Error())
	}
}

// Calling Delete() for a block that does not exist should result in error.
// Test should pass for both writable and read-only volumes
func testDeleteNoSuchBlock(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if err := v.Delete(TestHash2); err == nil {
		t.Errorf("Expected error when attempting to delete a non-existing block")
	}
}

// Invoke Status and verify that VolumeStatus is returned
// Test should pass for both writable and read-only volumes
func testStatus(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	// Get node status and make a basic sanity check.
	status := v.Status()
	if status.DeviceNum == 0 {
		t.Errorf("uninitialized device_num in %v", status)
	}

	if status.BytesFree == 0 {
		t.Errorf("uninitialized bytes_free in %v", status)
	}

	if status.BytesUsed == 0 {
		t.Errorf("uninitialized bytes_used in %v", status)
	}
}

// Invoke String for the volume; expect non-empty result
// Test should pass for both writable and read-only volumes
func testString(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if id := v.String(); len(id) == 0 {
		t.Error("Got empty string for v.String()")
	}
}

// Putting, updating, touching, and deleting blocks from a read-only volume result in error.
// Test is intended for only read-only volumes
func testUpdateReadOnly(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if v.Writable() == true {
		return
	}

	v.PutRaw(TestHash, TestBlock)

	// Get from read-only volume should succeed
	_, err := v.Get(TestHash)
	if err != nil {
		t.Errorf("got err %v, expected nil", err)
	}

	// Put a new block to read-only volume should result in error
	err = v.Put(TestHash2, TestBlock2)
	if err == nil {
		t.Errorf("Expected error when putting block in a read-only volume")
	}
	_, err = v.Get(TestHash2)
	if err == nil {
		t.Errorf("Expected error when getting block whose put in read-only volume failed")
	}

	// Touch a block in read-only volume should result in error
	err = v.Touch(TestHash)
	if err == nil {
		t.Errorf("Expected error when touching block in a read-only volume")
	}

	// Delete a block from a read-only volume should result in error
	err = v.Delete(TestHash)
	if err == nil {
		t.Errorf("Expected error when deleting block from a read-only volume")
	}

	// Overwriting an existing block in read-only volume should result in error
	err = v.Put(TestHash, TestBlock)
	if err == nil {
		t.Errorf("Expected error when putting block in a read-only volume")
	}
}

// Launch concurrent Gets
// Test should pass for both writable and read-only volumes
func testGetConcurrent(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	v.PutRaw(TestHash, TestBlock)
	v.PutRaw(TestHash2, TestBlock2)
	v.PutRaw(TestHash3, TestBlock3)

	sem := make(chan int)
	go func(sem chan int) {
		buf, err := v.Get(TestHash)
		if err != nil {
			t.Errorf("err1: %v", err)
		}
		bufs.Put(buf)
		if bytes.Compare(buf, TestBlock) != 0 {
			t.Errorf("buf should be %s, is %s", string(TestBlock), string(buf))
		}
		sem <- 1
	}(sem)

	go func(sem chan int) {
		buf, err := v.Get(TestHash2)
		if err != nil {
			t.Errorf("err2: %v", err)
		}
		bufs.Put(buf)
		if bytes.Compare(buf, TestBlock2) != 0 {
			t.Errorf("buf should be %s, is %s", string(TestBlock2), string(buf))
		}
		sem <- 1
	}(sem)

	go func(sem chan int) {
		buf, err := v.Get(TestHash3)
		if err != nil {
			t.Errorf("err3: %v", err)
		}
		bufs.Put(buf)
		if bytes.Compare(buf, TestBlock3) != 0 {
			t.Errorf("buf should be %s, is %s", string(TestBlock3), string(buf))
		}
		sem <- 1
	}(sem)

	// Wait for all goroutines to finish
	for done := 0; done < 3; {
		done += <-sem
	}
}

// Launch concurrent Puts
// Test is intended for only writable volumes
func testPutConcurrent(t *testing.T, factory TestableVolumeFactory) {
	v := factory(t)
	defer v.Teardown()

	if v.Writable() == false {
		return
	}

	sem := make(chan int)
	go func(sem chan int) {
		err := v.Put(TestHash, TestBlock)
		if err != nil {
			t.Errorf("err1: %v", err)
		}
		sem <- 1
	}(sem)

	go func(sem chan int) {
		err := v.Put(TestHash2, TestBlock2)
		if err != nil {
			t.Errorf("err2: %v", err)
		}
		sem <- 1
	}(sem)

	go func(sem chan int) {
		err := v.Put(TestHash3, TestBlock3)
		if err != nil {
			t.Errorf("err3: %v", err)
		}
		sem <- 1
	}(sem)

	// Wait for all goroutines to finish
	for done := 0; done < 3; {
		done += <-sem
	}

	// Double check that we actually wrote the blocks we expected to write.
	buf, err := v.Get(TestHash)
	if err != nil {
		t.Errorf("Get #1: %v", err)
	}
	bufs.Put(buf)
	if bytes.Compare(buf, TestBlock) != 0 {
		t.Errorf("Get #1: expected %s, got %s", string(TestBlock), string(buf))
	}

	buf, err = v.Get(TestHash2)
	if err != nil {
		t.Errorf("Get #2: %v", err)
	}
	bufs.Put(buf)
	if bytes.Compare(buf, TestBlock2) != 0 {
		t.Errorf("Get #2: expected %s, got %s", string(TestBlock2), string(buf))
	}

	buf, err = v.Get(TestHash3)
	if err != nil {
		t.Errorf("Get #3: %v", err)
	}
	bufs.Put(buf)
	if bytes.Compare(buf, TestBlock3) != 0 {
		t.Errorf("Get #3: expected %s, got %s", string(TestBlock3), string(buf))
	}
}