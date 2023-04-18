package cvsserr

import "errors"

var (
	ErrNullPointer            = errors.New("Null reference instance")
	ErrInvalidVector          = errors.New("invalid vector")
	ErrNotSupportVer          = errors.New("not support version")
	ErrNotSupportMetric       = errors.New("not support metric")
	ErrInvalidTemplate        = errors.New("invalid templete string")
	ErrSameMetric             = errors.New("exist same metric")
	ErrInvalidValue           = errors.New("invalid value of metric")
	ErrNoBaseMetrics          = errors.New("no Base metrics")
	ErrNoTemporalMetrics      = errors.New("no Temporal metrics")
	ErrNoEnvironmentalMetrics = errors.New("no Environmental metrics")
	ErrMisordered             = errors.New("misordered vector string")
)

/* Copyright 2018-2023 Spiegel
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 	http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
