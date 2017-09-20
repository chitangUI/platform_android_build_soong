// Copyright 2017 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package java

import (
	"android/soong/android"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var buildDir string

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_java_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	os.RemoveAll(buildDir)
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}

func testJava(t *testing.T, bp string) *android.TestContext {
	config := android.TestArchConfig(buildDir)

	ctx := android.NewTestArchContext()
	ctx.RegisterModuleType("android_app", android.ModuleFactoryAdaptor(AndroidAppFactory))
	ctx.RegisterModuleType("java_library", android.ModuleFactoryAdaptor(LibraryFactory))
	ctx.RegisterModuleType("java_library_host", android.ModuleFactoryAdaptor(LibraryHostFactory))
	ctx.RegisterModuleType("java_import", android.ModuleFactoryAdaptor(ImportFactory))
	ctx.RegisterModuleType("java_defaults", android.ModuleFactoryAdaptor(defaultsFactory))
	ctx.RegisterModuleType("android_prebuilt_sdk", android.ModuleFactoryAdaptor(SdkPrebuiltFactory))
	ctx.PreArchMutators(android.RegisterPrebuiltsPreArchMutators)
	ctx.PreArchMutators(android.RegisterPrebuiltsPostDepsMutators)
	ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
	ctx.Register()

	extraModules := []string{
		"core-oj",
		"core-libart",
		"framework",
		"ext",
		"okhttp",
		"android_stubs_current",
		"android_system_stubs_current",
		"android_test_stubs_current",
	}

	for _, extra := range extraModules {
		bp += fmt.Sprintf(`
			java_library {
				name: "%s",
				srcs: ["a.java"],
				no_standard_libs: true,
			}
		`, extra)
	}

	bp += `
		android_prebuilt_sdk {
			name: "sdk_v14",
			jars: ["sdk_v14.jar"],
		}
	`

	ctx.MockFileSystem(map[string][]byte{
		"Android.bp":  []byte(bp),
		"a.java":      nil,
		"b.java":      nil,
		"c.java":      nil,
		"a.jar":       nil,
		"b.jar":       nil,
		"sdk_v14.jar": nil,
	})

	_, errs := ctx.ParseBlueprintsFiles("Android.bp")
	fail(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	fail(t, errs)

	return ctx
}

func moduleToPath(name string) string {
	switch {
	case name == `""`:
		return name
	case strings.HasPrefix(name, "sdk_v"):
		return name + ".jar"
	default:
		return filepath.Join(buildDir, ".intermediates", name, "android_common", "classes-desugar.jar")
	}
}

func TestSimple(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			static_libs: ["baz"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
		}

		java_library {
			name: "baz",
			srcs: ["c.java"],
		}
		`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	combineJar := ctx.ModuleForTests("foo", "android_common").Rule("combineJar")

	if len(javac.Inputs) != 1 || javac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, javac.Inputs)
	}

	bar := filepath.Join(buildDir, ".intermediates", "bar", "android_common", "classes-desugar.jar")
	baz := filepath.Join(buildDir, ".intermediates", "baz", "android_common", "classes-desugar.jar")

	if !strings.Contains(javac.Args["classpath"], bar) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], bar)
	}

	if !strings.Contains(javac.Args["classpath"], baz) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], baz)
	}

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != baz {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, baz)
	}
}

var classpathTestcases = []struct {
	name          string
	host          android.OsClass
	properties    string
	bootclasspath []string
	classpath     []string
}{
	{
		name:          "default",
		bootclasspath: []string{"core-oj", "core-libart"},
		classpath:     []string{"ext", "framework", "okhttp"},
	},
	{
		name:          "blank sdk version",
		properties:    `sdk_version: "",`,
		bootclasspath: []string{"core-oj", "core-libart"},
		classpath:     []string{"ext", "framework", "okhttp"},
	},
	{

		name:          "sdk v14",
		properties:    `sdk_version: "14",`,
		bootclasspath: []string{"sdk_v14"},
		classpath:     []string{},
	},
	{

		name:          "current",
		properties:    `sdk_version: "current",`,
		bootclasspath: []string{"android_stubs_current"},
		classpath:     []string{},
	},
	{

		name:          "system_current",
		properties:    `sdk_version: "system_current",`,
		bootclasspath: []string{"android_system_stubs_current"},
		classpath:     []string{},
	},
	{

		name:          "test_current",
		properties:    `sdk_version: "test_current",`,
		bootclasspath: []string{"android_test_stubs_current"},
		classpath:     []string{},
	},
	{

		name:          "nostdlib",
		properties:    `no_standard_libs: true`,
		bootclasspath: []string{`""`},
		classpath:     []string{},
	},
	{

		name:       "host default",
		host:       android.Host,
		properties: ``,
		classpath:  []string{},
	},
	{
		name:       "host nostdlib",
		host:       android.Host,
		properties: `no_standard_libs: true`,
		classpath:  []string{},
	},
}

func TestClasspath(t *testing.T) {
	for _, testcase := range classpathTestcases {
		t.Run(testcase.name, func(t *testing.T) {
			hostExtra := ""
			if testcase.host == android.Host {
				hostExtra = "_host"
			}
			ctx := testJava(t, `
			java_library`+hostExtra+` {
				name: "foo",
				srcs: ["a.java"],
				`+testcase.properties+`
			}
			`)

			convertModulesToPaths := func(cp []string) []string {
				ret := make([]string, len(cp))
				for i, e := range cp {
					ret[i] = moduleToPath(e)
				}
				return ret
			}

			bootclasspath := convertModulesToPaths(testcase.bootclasspath)
			classpath := convertModulesToPaths(testcase.classpath)

			variant := "android_common"
			if testcase.host == android.Host {
				variant = android.BuildOs.String() + "_common"
			}
			javac := ctx.ModuleForTests("foo", variant).Rule("javac")

			got := strings.TrimPrefix(javac.Args["bootClasspath"], "-bootclasspath ")
			bc := strings.Join(bootclasspath, ":")
			if got != bc {
				t.Errorf("bootclasspath expected %q != got %q", bc, got)
			}

			got = strings.TrimPrefix(javac.Args["classpath"], "-classpath ")
			c := strings.Join(classpath, ":")
			if got != c {
				t.Errorf("classpath expected %q != got %q", c, got)
			}

			var deps []string
			if len(bootclasspath) > 0 && bootclasspath[0] != `""` {
				deps = append(deps, bootclasspath...)
			}
			deps = append(deps, classpath...)

			if !reflect.DeepEqual(javac.Implicits.Strings(), deps) {
				t.Errorf("implicits expected %q != got %q", deps, javac.Implicits.Strings())
			}
		})
	}

}

func TestPrebuilts(t *testing.T) {
	ctx := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			libs: ["bar"],
			static_libs: ["baz"],
		}

		java_import {
			name: "bar",
			jars: ["a.jar"],
		}

		java_import {
			name: "baz",
			jars: ["b.jar"],
		}
		`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	combineJar := ctx.ModuleForTests("foo", "android_common").Rule("combineJar")

	bar := "a.jar"
	if !strings.Contains(javac.Args["classpath"], bar) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], bar)
	}

	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != "b.jar" {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, "b.jar")
	}
}

func TestDefaults(t *testing.T) {
	ctx := testJava(t, `
		java_defaults {
			name: "defaults",
			srcs: ["a.java"],
			libs: ["bar"],
			static_libs: ["baz"],
		}

		java_library {
			name: "foo",
			defaults: ["defaults"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
		}

		java_library {
			name: "baz",
			srcs: ["c.java"],
		}
		`)

	javac := ctx.ModuleForTests("foo", "android_common").Rule("javac")
	combineJar := ctx.ModuleForTests("foo", "android_common").Rule("combineJar")

	if len(javac.Inputs) != 1 || javac.Inputs[0].String() != "a.java" {
		t.Errorf(`foo inputs %v != ["a.java"]`, javac.Inputs)
	}

	bar := filepath.Join(buildDir, ".intermediates", "bar", "android_common", "classes-desugar.jar")
	if !strings.Contains(javac.Args["classpath"], bar) {
		t.Errorf("foo classpath %v does not contain %q", javac.Args["classpath"], bar)
	}

	baz := filepath.Join(buildDir, ".intermediates", "baz", "android_common", "classes-desugar.jar")
	if len(combineJar.Inputs) != 2 || combineJar.Inputs[1].String() != baz {
		t.Errorf("foo combineJar inputs %v does not contain %q", combineJar.Inputs, baz)
	}
}

func fail(t *testing.T, errs []error) {
	if len(errs) > 0 {
		for _, err := range errs {
			t.Error(err)
		}
		t.FailNow()
	}
}
