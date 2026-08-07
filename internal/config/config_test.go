package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// setEnv clears every variable in cleared, then applies env on top. Tests clear the whole group they
// exercise — rather than only the variables a case sets — so a value left over in the developer's
// real environment or in .env can't change a result. t.Setenv restores everything when the test ends.
func setEnv(t *testing.T, cleared []string, env map[string]string) {
	t.Helper()

	for _, name := range cleared {
		t.Setenv(name, "")
	}

	for name, value := range env {
		t.Setenv(name, value)
	}
}

// unsetEnv removes name for the duration of the test. t.Setenv on its own can't express this — it
// only assigns — but it does register the restore, so unsetting after it leaves the original value
// to be put back at cleanup. The distinction matters here: godotenv skips any variable that is
// present, so a variable set to "" is not the same as one that is absent.
func unsetEnv(t *testing.T, name string) {
	t.Helper()

	t.Setenv(name, "")

	require.NoError(t, os.Unsetenv(name))
}

// writeDotEnv drops a .env file into a temporary directory and makes it the working one, so LoadEnv
// (which reads ./.env) picks it up without touching the repository's own.
func writeDotEnv(t *testing.T, contents string) {
	t.Helper()

	t.Chdir(t.TempDir())

	require.NoError(t, os.WriteFile(".env", []byte(contents), 0o600))
}

// TestLoadEnv_ReadsDotEnv checks the local-development convenience: values in a .env file reach the
// process environment without anyone having to export them.
func TestLoadEnv_ReadsDotEnv(t *testing.T) {
	unsetEnv(t, "CONFIG_TEST_FROM_FILE")
	writeDotEnv(t, "CONFIG_TEST_FROM_FILE=from-file\n")

	LoadEnv()

	require.Equal(t, "from-file", os.Getenv("CONFIG_TEST_FROM_FILE"))
}

// TestLoadEnv_EmptyVariableStillCounts documents a sharp edge worth knowing before debugging one:
// godotenv skips names that are already present in the environment, and a variable exported as an
// empty string is present. Setting FOO= in a shell or a compose file therefore doesn't fall back to
// .env — it pins the value to empty.
func TestLoadEnv_EmptyVariableStillCounts(t *testing.T) {
	t.Setenv("CONFIG_TEST_EMPTY", "")
	writeDotEnv(t, "CONFIG_TEST_EMPTY=from-file\n")

	LoadEnv()

	require.Empty(t, os.Getenv("CONFIG_TEST_EMPTY"), "want the empty environment value to stand")
}

// TestLoadEnv_RealEnvironmentWins is the contract that makes .env safe to ship: in production, where
// configuration arrives as real environment variables, a stray .env must not override it.
func TestLoadEnv_RealEnvironmentWins(t *testing.T) {
	t.Setenv("CONFIG_TEST_PRECEDENCE", "from-environment")
	writeDotEnv(t, "CONFIG_TEST_PRECEDENCE=from-file\n")

	LoadEnv()

	require.Equal(t, "from-environment", os.Getenv("CONFIG_TEST_PRECEDENCE"), "want the real environment to win")
}

// TestLoadEnv_NoDotEnv checks that a missing .env is the normal case, not a failure: production runs
// entirely on real environment variables.
func TestLoadEnv_NoDotEnv(t *testing.T) {
	t.Chdir(t.TempDir())

	LoadEnv() // must not panic or exit
}

// TestStringEnv covers the rule every other reader inherits: trim, and treat what's left of a blank
// value as unset.
func TestStringEnv(t *testing.T) {
	tests := map[string]struct {
		value string
		want  string
	}{
		"unset": {
			value: "",
			want:  "fallback",
		},
		"blank": {
			value: "   ",
			want:  "fallback",
		},
		"trimmed": {
			value: "  value  ",
			want:  "value",
		},
		"inner spaces kept": {
			value: " two words ",
			want:  "two words",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CONFIG_TEST_STRING", tt.value)

			require.Equal(t, tt.want, stringEnv("CONFIG_TEST_STRING", "fallback"))
		})
	}
}

// TestTypedEnvReaders checks that the parsers agree with stringEnv on what a value is — a padded
// number is a number, a blank one is unset — so a stray space in .env can't turn into a startup
// error that names the wrong problem.
func TestTypedEnvReaders(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		tests := map[string]struct {
			value   string
			want    int
			wantErr bool
		}{
			"unset": {
				value: "",
				want:  7,
			},
			"blank": {
				value: "  ",
				want:  7,
			},
			"padded": {
				value: " 25 ",
				want:  25,
			},
			"negative": {
				value: "-3",
				want:  -3,
			},
			"not a number": {
				value:   "lots",
				wantErr: true,
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				t.Setenv("CONFIG_TEST_INT", tt.value)

				got, err := intEnv("CONFIG_TEST_INT", 7)
				if tt.wantErr {
					require.Error(t, err)

					return
				}

				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("bool", func(t *testing.T) {
		tests := map[string]struct {
			value   string
			want    bool
			wantErr bool
		}{
			"unset": {
				value: "",
				want:  true,
			},
			"blank": {
				value: " ",
				want:  true,
			},
			"padded": {
				value: " false ",
				want:  false,
			},
			"one": {
				value: "1",
				want:  true,
			},
			"not a bool": {
				value:   "yes",
				wantErr: true,
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				t.Setenv("CONFIG_TEST_BOOL", tt.value)

				got, err := boolEnv("CONFIG_TEST_BOOL", true)
				if tt.wantErr {
					require.Error(t, err)

					return
				}

				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("duration", func(t *testing.T) {
		tests := map[string]struct {
			value   string
			want    time.Duration
			wantErr bool
		}{
			"unset": {
				value: "",
				want:  time.Minute,
			},
			"blank": {
				value: "\t",
				want:  time.Minute,
			},
			"padded": {
				value: " 90s ",
				want:  90 * time.Second,
			},
			"no unit": {
				value:   "90",
				wantErr: true,
			},
			"not a duration": {
				value:   "soon",
				wantErr: true,
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				t.Setenv("CONFIG_TEST_DURATION", tt.value)

				got, err := durationEnv("CONFIG_TEST_DURATION", time.Minute)
				if tt.wantErr {
					require.Error(t, err)

					return
				}

				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			})
		}
	})
}

// TestPortEnv_Bounds pins the edges of the accepted range, which the "must be between 1 and 65535"
// message promises: both ends are usable, and stepping past either is an error.
func TestPortEnv_Bounds(t *testing.T) {
	tests := map[string]struct {
		value   string
		wantErr bool
	}{
		"lowest":  {value: "1"},
		"highest": {value: "65535"},
		"zero": {
			value:   "0",
			wantErr: true,
		},
		"above range": {
			value:   "65536",
			wantErr: true,
		},
		"negative": {
			value:   "-1",
			wantErr: true,
		},
		"not a number": {
			value:   "http",
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CONFIG_TEST_PORT", tt.value)

			got, err := portEnv("CONFIG_TEST_PORT", "8080")
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.value, got)
		})
	}
}
