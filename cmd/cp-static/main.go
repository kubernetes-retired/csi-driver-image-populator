/*
Copyright 2019 The Kubernetes Authors.

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

package main

import (
	"fmt"
	"os"

	. "github.com/otiai10/copy"
)

func main() {
	if len(os.Args) < 3 {
		os.Exit(1)
	}
	if err := Copy(os.Args[1], os.Args[2]); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(0)
}
