/*
 *     Copyright 2024 The CNAI Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1

import "time"

const (
	// AnnotationFilepath is the annotation key for the file path of the layer.
	AnnotationFilepath = "org.cnai.model.filepath"

	// AnnotationFileMetadata is the annotation key for the file metadata of the layer.
	AnnotationFileMetadata = "org.cnai.model.file.metadata+json"

	// AnnotationUntested is the annotation key for file media type untested flag of the layer.
	AnnotationMediaTypeUntested = "org.cnai.model.file.mediatype.untested"
)

// FileMetadata represents the metadata of file, which is the value definition of AnnotationFileMetadata.
type FileMetadata struct {
	// File name
	Name string `json:"name"`

	// File permission mode (e.g., Unix permission bits)
	Mode uint32 `json:"mode"`

	// User ID (identifier of the file owner)
	Uid uint32 `json:"uid"`

	// Group ID (identifier of the file's group)
	Gid uint32 `json:"gid"`

	// File size (in bytes)
	Size int64 `json:"size"`

	// File last modification time
	ModTime time.Time `json:"mtime"`

	// File type flag (e.g., regular file, directory, etc.)
	Typeflag byte `json:"typeflag"`
}
