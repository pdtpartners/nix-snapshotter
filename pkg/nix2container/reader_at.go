/*
   Copyright The BuildKit Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package nix2container

import (
	"io"

	"github.com/pkg/errors"
)

// readerAt is from https://github.com/moby/buildkit/blob/v0.10.6/util/contentutil/fetcher.go#L32-L73 (Apache License 2.0)
type readerAt struct {
	io.Reader
	io.Closer
	size   int64
	offset int64
}

func (r *readerAt) ReadAt(b []byte, off int64) (int, error) {
	if ra, ok := r.Reader.(io.ReaderAt); ok {
		return ra.ReadAt(b, off)
	}

	if r.offset != off {
		if seeker, ok := r.Reader.(io.Seeker); ok {
			if _, err := seeker.Seek(off, io.SeekStart); err != nil {
				return 0, err
			}
			r.offset = off
		} else {
			return 0, errors.Errorf("unsupported offset")
		}
	}

	var totalN int
	for len(b) > 0 {
		n, err := r.Reader.Read(b)
		if err == io.EOF && n == len(b) {
			err = nil
		}
		r.offset += int64(n)
		totalN += n
		b = b[n:]
		if err != nil {
			return totalN, err
		}
	}
	return totalN, nil
}

func (r *readerAt) Size() int64 {
	return r.size
}
