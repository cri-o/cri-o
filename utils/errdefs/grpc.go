/*
   Copyright The containerd Authors.

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

package errdefs

import (
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ToGRPC will attempt to map the backend containerd error into a grpc error,
// using the original error message as a description.
//
// Further information may be extracted from certain errors depending on their
// type.
//
// If the error is unmapped, the original error will be returned to be handled
// by the regular grpc error handling stack.
func ToGRPC(err error) error {
	if err == nil {
		return nil
	}

	if isGRPCError(err) {
		// error has already been mapped to grpc
		return err
	}

	switch {
	case IsInvalidArgument(err):
		return status.Errorf(codes.InvalidArgument, "%v", err)
	case IsNotFound(err):
		return status.Errorf(codes.NotFound, "%v", err)
	case IsAlreadyExists(err):
		return status.Errorf(codes.AlreadyExists, "%v", err)
	case IsFailedPrecondition(err):
		return status.Errorf(codes.FailedPrecondition, "%v", err)
	case IsUnavailable(err):
		return status.Errorf(codes.Unavailable, "%v", err)
	case IsNotImplemented(err):
		return status.Errorf(codes.Unimplemented, "%v", err)
	}

	return err
}

// ToGRPCf maps the error to grpc error codes, assembling the formatting string
// and combining it with the target error string.
func ToGRPCf(err error, format string, args ...any) error {
	return ToGRPC(fmt.Errorf(format+": %w", append(args, err)...))
}

// FromGRPC returns the underlying error from a grpc service based on the grpc error code.
func FromGRPC(err error) error {
	if err == nil {
		return nil
	}

	var cls error // divide these into error classes, becomes the cause

	switch code(err) {
	case codes.InvalidArgument:
		cls = ErrInvalidArgument
	case codes.AlreadyExists:
		cls = ErrAlreadyExists
	case codes.NotFound:
		cls = ErrNotFound
	case codes.Unavailable:
		cls = ErrUnavailable
	case codes.FailedPrecondition:
		cls = ErrFailedPrecondition
	case codes.Unimplemented:
		cls = ErrNotImplemented
	default:
		cls = ErrUnknown
	}

	msg := rebaseMessage(cls, err)
	if msg != "" {
		err = fmt.Errorf(msg+": %w", cls)
	} else {
		err = cls
	}

	return err
}

// rebaseMessage removes the repeats for an error at the end of an error
// string. This will happen when taking an error over grpc then remapping it.
//
// Effectively, we just remove the string of cls from the end of err if it
// appears there.
func rebaseMessage(cls, err error) string {
	desc := errDesc(err)
	clss := cls.Error()

	if desc == clss {
		return ""
	}

	return strings.TrimSuffix(desc, ": "+clss)
}

func isGRPCError(err error) bool {
	_, ok := status.FromError(err)

	return ok
}

func code(err error) codes.Code {
	if s, ok := status.FromError(err); ok {
		return s.Code()
	}

	return codes.Unknown
}

func errDesc(err error) string {
	if s, ok := status.FromError(err); ok {
		return s.Message()
	}

	return err.Error()
}
