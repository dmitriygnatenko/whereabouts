package login

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/domain/entity"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
	"wherewhat/internal/port/mocks"
)

// TestLoginUserUsecase_Execute covers UseCase.Execute end to end against mocked dependencies: each
// case builds and stubs its own mocks, wiring up only the calls that scenario should reach, so an
// unexpected call (e.g. UpdateLanguage running when a saved language should be left alone) fails
// the test on its own via gomock.
func TestLoginUserUsecase_Execute(t *testing.T) {
	supportedLanguages := make([]string, 0, len(entity.SupportedLanguages))
	for lang := range entity.SupportedLanguages {
		supportedLanguages = append(supportedLanguages, lang)
	}

	// unsupportedLanguage is a two-letter code guaranteed not to be one of entity.SupportedLanguages.
	var unsupportedLanguage string

	for {
		candidate := strings.ToLower(gofakeit.LetterN(2))
		if _, ok := entity.SupportedLanguages[candidate]; !ok {
			unsupportedLanguage = candidate

			break
		}
	}

	type testCase struct {
		name             string
		input            Input
		userRepoMock     func(t *testing.T, mc *gomock.Controller) port.UserRepository
		sessionRepoMock  func(t *testing.T, mc *gomock.Controller) port.SessionRepository
		hasherMock       func(t *testing.T, mc *gomock.Controller) port.PasswordHasher
		tokenGenMock     func(t *testing.T, mc *gomock.Controller) port.TokenGenerator
		wantErrMsg       string // exact err.Error(); empty means Execute must return nil
		wantUnauthorized bool   // also assert err is a *domainerror.UnauthorizedError
		// check runs extra assertions on a successful Output; unused when wantErrMsg is set.
		check func(t *testing.T, out Output)
	}

	var tests []testCase
	{ // success adopts a supported language and normalizes the username
		username := strings.ToLower(strings.ReplaceAll(gofakeit.Username(), " ", ""))
		rawUsername := "  " + strings.ToUpper(username) + "  "
		password := gofakeit.Password(true, true, true, false, false, 16)
		passwordHash := gofakeit.Password(true, true, true, true, false, 60)
		userID := uint64(gofakeit.Uint32())
		sessionToken := gofakeit.UUID()
		language := gofakeit.RandomString(supportedLanguages)

		tests = append(tests, testCase{
			name:  "success adopts a supported language and normalizes the username",
			input: Input{Username: rawUsername, Password: password, Language: strings.ToUpper(language)},
			userRepoMock: func(t *testing.T, mc *gomock.Controller) port.UserRepository {
				mock := mocks.NewMockUserRepository(mc)

				user := entity.User{ID: userID, Username: username, PasswordHash: passwordHash}
				mock.EXPECT().FindByUsername(context.Background(), username).Return(user, nil)
				mock.EXPECT().UpdateLanguage(context.Background(), userID, language).Return(nil)

				return mock
			},
			sessionRepoMock: func(t *testing.T, mc *gomock.Controller) port.SessionRepository {
				mock := mocks.NewMockSessionRepository(mc)

				mock.EXPECT().Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, session entity.Session) error {
						if session.Token != sessionToken || session.UserID != userID {
							t.Fatalf("SessionRepository.Create got %+v, want token %s, user %d",
								session, sessionToken, userID)
						}

						return nil
					})

				return mock
			},
			hasherMock: func(t *testing.T, mc *gomock.Controller) port.PasswordHasher {
				mock := mocks.NewMockPasswordHasher(mc)
				mock.EXPECT().Compare(passwordHash, password).Return(true)

				return mock
			},
			tokenGenMock: func(t *testing.T, mc *gomock.Controller) port.TokenGenerator {
				mock := mocks.NewMockTokenGenerator(mc)
				mock.EXPECT().NewToken().Return(sessionToken, nil)

				return mock
			},
			check: func(t *testing.T, out Output) {
				t.Helper()

				if out.User.ID != userID || out.User.Username != username || out.User.Language != language {
					t.Fatalf("Execute() User = %+v, want id %d, username %s, language %s",
						out.User, userID, username, language)
				}

				if out.Session.Token != sessionToken || out.Session.UserID != userID {
					t.Fatalf("Execute() Session = %+v, want token %s, user %d", out.Session, sessionToken, userID)
				}
			},
		})
	}

	{ // existing language is not overwritten
		username := strings.ToLower(strings.ReplaceAll(gofakeit.Username(), " ", ""))
		password := gofakeit.Password(true, true, true, false, false, 16)
		passwordHash := gofakeit.Password(true, true, true, true, false, 60)
		userID := uint64(gofakeit.Uint32())
		sessionToken := gofakeit.UUID()
		existingLanguage := gofakeit.RandomString(supportedLanguages)
		attemptedLanguage := gofakeit.RandomString(supportedLanguages)

		tests = append(tests, testCase{
			name:  "existing language is not overwritten",
			input: Input{Username: username, Password: password, Language: attemptedLanguage},
			userRepoMock: func(t *testing.T, mc *gomock.Controller) port.UserRepository {
				mock := mocks.NewMockUserRepository(mc)

				user := entity.User{
					ID: userID, Username: username, PasswordHash: passwordHash,
					Settings: entity.UserSettings{Language: existingLanguage},
				}
				mock.EXPECT().FindByUsername(context.Background(), username).Return(user, nil)
				// UpdateLanguage is deliberately left unstubbed: any call to it fails the test.

				return mock
			},
			sessionRepoMock: func(t *testing.T, mc *gomock.Controller) port.SessionRepository {
				mock := mocks.NewMockSessionRepository(mc)
				mock.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

				return mock
			},
			hasherMock: func(t *testing.T, mc *gomock.Controller) port.PasswordHasher {
				mock := mocks.NewMockPasswordHasher(mc)
				mock.EXPECT().Compare(passwordHash, password).Return(true)

				return mock
			},
			tokenGenMock: func(t *testing.T, mc *gomock.Controller) port.TokenGenerator {
				mock := mocks.NewMockTokenGenerator(mc)
				mock.EXPECT().NewToken().Return(sessionToken, nil)

				return mock
			},
			check: func(t *testing.T, out Output) {
				t.Helper()

				if out.User.Language != existingLanguage {
					t.Fatalf("Execute() User.Language = %q, want %q", out.User.Language, existingLanguage)
				}
			},
		})
	}

	{ // unsupported language is not saved
		username := strings.ToLower(strings.ReplaceAll(gofakeit.Username(), " ", ""))
		password := gofakeit.Password(true, true, true, false, false, 16)
		passwordHash := gofakeit.Password(true, true, true, true, false, 60)
		userID := uint64(gofakeit.Uint32())
		sessionToken := gofakeit.UUID()

		tests = append(tests, testCase{
			name:  "unsupported language is not saved",
			input: Input{Username: username, Password: password, Language: unsupportedLanguage},
			userRepoMock: func(t *testing.T, mc *gomock.Controller) port.UserRepository {
				mock := mocks.NewMockUserRepository(mc)

				user := entity.User{ID: userID, Username: username, PasswordHash: passwordHash}
				mock.EXPECT().FindByUsername(context.Background(), username).Return(user, nil)
				// UpdateLanguage is deliberately left unstubbed: any call to it fails the test.

				return mock
			},
			sessionRepoMock: func(t *testing.T, mc *gomock.Controller) port.SessionRepository {
				mock := mocks.NewMockSessionRepository(mc)
				mock.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

				return mock
			},
			hasherMock: func(t *testing.T, mc *gomock.Controller) port.PasswordHasher {
				mock := mocks.NewMockPasswordHasher(mc)
				mock.EXPECT().Compare(passwordHash, password).Return(true)

				return mock
			},
			tokenGenMock: func(t *testing.T, mc *gomock.Controller) port.TokenGenerator {
				mock := mocks.NewMockTokenGenerator(mc)
				mock.EXPECT().NewToken().Return(sessionToken, nil)

				return mock
			},
			check: func(t *testing.T, out Output) {
				t.Helper()

				if out.User.Language != "" {
					t.Fatalf("Execute() User.Language = %q, want empty", out.User.Language)
				}
			},
		})
	}

	{ // an empty username is rejected before any repository lookup
		password := gofakeit.Password(true, true, true, false, false, 16)

		tests = append(tests, testCase{
			name:  "an empty username is rejected before any repository lookup",
			input: Input{Username: "", Password: password},
			userRepoMock: func(t *testing.T, mc *gomock.Controller) port.UserRepository {
				// Deliberately left unstubbed: validation must short-circuit before the lookup.
				return mocks.NewMockUserRepository(mc)
			},
			sessionRepoMock: func(t *testing.T, mc *gomock.Controller) port.SessionRepository {
				return mocks.NewMockSessionRepository(mc)
			},
			hasherMock: func(t *testing.T, mc *gomock.Controller) port.PasswordHasher {
				return mocks.NewMockPasswordHasher(mc)
			},
			tokenGenMock: func(t *testing.T, mc *gomock.Controller) port.TokenGenerator {
				return mocks.NewMockTokenGenerator(mc)
			},
			wantErrMsg: "Please enter a username",
		})
	}

	{ // an empty password is rejected before any repository lookup
		username := strings.ToLower(strings.ReplaceAll(gofakeit.Username(), " ", ""))

		tests = append(tests, testCase{
			name:  "an empty password is rejected before any repository lookup",
			input: Input{Username: username, Password: ""},
			// Required fires before the Length rule, so a truly empty password gets this message
			// rather than the "at least N characters" one a too-short-but-non-empty password would.
			userRepoMock: func(t *testing.T, mc *gomock.Controller) port.UserRepository {
				// Deliberately left unstubbed: validation must short-circuit before the lookup.
				return mocks.NewMockUserRepository(mc)
			},
			sessionRepoMock: func(t *testing.T, mc *gomock.Controller) port.SessionRepository {
				return mocks.NewMockSessionRepository(mc)
			},
			hasherMock: func(t *testing.T, mc *gomock.Controller) port.PasswordHasher {
				return mocks.NewMockPasswordHasher(mc)
			},
			tokenGenMock: func(t *testing.T, mc *gomock.Controller) port.TokenGenerator {
				return mocks.NewMockTokenGenerator(mc)
			},
			wantErrMsg: "Please enter a password",
		})
	}

	{ // unknown username is rejected without comparing a password
		username := strings.ToLower(strings.ReplaceAll(gofakeit.Username(), " ", ""))
		password := gofakeit.Password(true, true, true, false, false, 16)
		lookupErr := gofakeit.Error()

		tests = append(tests, testCase{
			name:  "unknown username is rejected without comparing a password",
			input: Input{Username: username, Password: password},
			userRepoMock: func(t *testing.T, mc *gomock.Controller) port.UserRepository {
				mock := mocks.NewMockUserRepository(mc)
				mock.EXPECT().FindByUsername(context.Background(), username).Return(entity.User{}, lookupErr)

				return mock
			},
			sessionRepoMock: func(t *testing.T, mc *gomock.Controller) port.SessionRepository {
				return mocks.NewMockSessionRepository(mc)
			},
			hasherMock: func(t *testing.T, mc *gomock.Controller) port.PasswordHasher {
				// Deliberately left unstubbed: the lookup failing must short-circuit before Compare.
				return mocks.NewMockPasswordHasher(mc)
			},
			tokenGenMock: func(t *testing.T, mc *gomock.Controller) port.TokenGenerator {
				return mocks.NewMockTokenGenerator(mc)
			},
			wantErrMsg:       "Incorrect username or password",
			wantUnauthorized: true,
		})
	}

	{ // wrong password is rejected with the same message as an unknown username
		username := strings.ToLower(strings.ReplaceAll(gofakeit.Username(), " ", ""))
		passwordHash := gofakeit.Password(true, true, true, true, false, 60)
		wrongPassword := gofakeit.Password(true, true, true, false, false, 16)
		userID := uint64(gofakeit.Uint32())

		tests = append(tests, testCase{
			name:  "wrong password is rejected with the same message as an unknown username",
			input: Input{Username: username, Password: wrongPassword},
			userRepoMock: func(t *testing.T, mc *gomock.Controller) port.UserRepository {
				mock := mocks.NewMockUserRepository(mc)

				user := entity.User{ID: userID, Username: username, PasswordHash: passwordHash}
				mock.EXPECT().FindByUsername(context.Background(), username).Return(user, nil)

				return mock
			},
			sessionRepoMock: func(t *testing.T, mc *gomock.Controller) port.SessionRepository {
				return mocks.NewMockSessionRepository(mc)
			},
			hasherMock: func(t *testing.T, mc *gomock.Controller) port.PasswordHasher {
				mock := mocks.NewMockPasswordHasher(mc)
				mock.EXPECT().Compare(passwordHash, wrongPassword).Return(false)

				return mock
			},
			tokenGenMock: func(t *testing.T, mc *gomock.Controller) port.TokenGenerator {
				return mocks.NewMockTokenGenerator(mc)
			},
			wantErrMsg:       "Incorrect username or password",
			wantUnauthorized: true,
		})
	}

	{ // a failure saving the detected language aborts the login
		username := strings.ToLower(strings.ReplaceAll(gofakeit.Username(), " ", ""))
		password := gofakeit.Password(true, true, true, false, false, 16)
		passwordHash := gofakeit.Password(true, true, true, true, false, 60)
		userID := uint64(gofakeit.Uint32())
		language := gofakeit.RandomString(supportedLanguages)
		saveErr := gofakeit.Error()

		tests = append(tests, testCase{
			name:  "a failure saving the detected language aborts the login",
			input: Input{Username: username, Password: password, Language: language},
			userRepoMock: func(t *testing.T, mc *gomock.Controller) port.UserRepository {
				mock := mocks.NewMockUserRepository(mc)

				user := entity.User{ID: userID, Username: username, PasswordHash: passwordHash}
				mock.EXPECT().FindByUsername(context.Background(), username).Return(user, nil)
				mock.EXPECT().UpdateLanguage(context.Background(), userID, language).Return(saveErr)

				return mock
			},
			sessionRepoMock: func(t *testing.T, mc *gomock.Controller) port.SessionRepository {
				// Deliberately left unstubbed: a language save failure must return before it.
				return mocks.NewMockSessionRepository(mc)
			},
			hasherMock: func(t *testing.T, mc *gomock.Controller) port.PasswordHasher {
				mock := mocks.NewMockPasswordHasher(mc)
				mock.EXPECT().Compare(passwordHash, password).Return(true)

				return mock
			},
			tokenGenMock: func(t *testing.T, mc *gomock.Controller) port.TokenGenerator {
				// Deliberately left unstubbed: a language save failure must return before it.
				return mocks.NewMockTokenGenerator(mc)
			},
			wantErrMsg: "Failed to save language preference",
		})
	}

	{ // a token generation failure aborts the login before a session is persisted
		username := strings.ToLower(strings.ReplaceAll(gofakeit.Username(), " ", ""))
		password := gofakeit.Password(true, true, true, false, false, 16)
		passwordHash := gofakeit.Password(true, true, true, true, false, 60)
		userID := uint64(gofakeit.Uint32())
		tokenErr := gofakeit.Error()

		tests = append(tests, testCase{
			name:  "a token generation failure aborts the login before a session is persisted",
			input: Input{Username: username, Password: password},
			userRepoMock: func(t *testing.T, mc *gomock.Controller) port.UserRepository {
				mock := mocks.NewMockUserRepository(mc)

				user := entity.User{ID: userID, Username: username, PasswordHash: passwordHash}
				mock.EXPECT().FindByUsername(context.Background(), username).Return(user, nil)

				return mock
			},
			sessionRepoMock: func(t *testing.T, mc *gomock.Controller) port.SessionRepository {
				// Deliberately left unstubbed: a token error must return before it.
				return mocks.NewMockSessionRepository(mc)
			},
			hasherMock: func(t *testing.T, mc *gomock.Controller) port.PasswordHasher {
				mock := mocks.NewMockPasswordHasher(mc)
				mock.EXPECT().Compare(passwordHash, password).Return(true)

				return mock
			},
			tokenGenMock: func(t *testing.T, mc *gomock.Controller) port.TokenGenerator {
				mock := mocks.NewMockTokenGenerator(mc)
				mock.EXPECT().NewToken().Return("", tokenErr)

				return mock
			},
			wantErrMsg: "Failed to start a session",
		})
	}

	{ // a session persistence failure gets the same message as a token generation failure
		username := strings.ToLower(strings.ReplaceAll(gofakeit.Username(), " ", ""))
		password := gofakeit.Password(true, true, true, false, false, 16)
		passwordHash := gofakeit.Password(true, true, true, true, false, 60)
		userID := uint64(gofakeit.Uint32())
		sessionToken := gofakeit.UUID()
		createErr := gofakeit.Error()

		tests = append(tests, testCase{
			name:  "a session persistence failure gets the same message as a token generation failure",
			input: Input{Username: username, Password: password},
			userRepoMock: func(t *testing.T, mc *gomock.Controller) port.UserRepository {
				mock := mocks.NewMockUserRepository(mc)

				user := entity.User{ID: userID, Username: username, PasswordHash: passwordHash}
				mock.EXPECT().FindByUsername(context.Background(), username).Return(user, nil)

				return mock
			},
			sessionRepoMock: func(t *testing.T, mc *gomock.Controller) port.SessionRepository {
				mock := mocks.NewMockSessionRepository(mc)
				mock.EXPECT().Create(gomock.Any(), gomock.Any()).Return(createErr)

				return mock
			},
			hasherMock: func(t *testing.T, mc *gomock.Controller) port.PasswordHasher {
				mock := mocks.NewMockPasswordHasher(mc)
				mock.EXPECT().Compare(passwordHash, password).Return(true)

				return mock
			},
			tokenGenMock: func(t *testing.T, mc *gomock.Controller) port.TokenGenerator {
				mock := mocks.NewMockTokenGenerator(mc)
				mock.EXPECT().NewToken().Return(sessionToken, nil)

				return mock
			},
			wantErrMsg: "Failed to start a session",
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := gomock.NewController(t)
			t.Cleanup(mc.Finish)

			uc := New(tt.userRepoMock(t, mc), tt.sessionRepoMock(t, mc), tt.hasherMock(t, mc), tt.tokenGenMock(t, mc))

			out, err := uc.Execute(context.Background(), tt.input)

			if tt.wantErrMsg == "" {
				if err != nil {
					t.Fatalf("Execute() error = %v, want nil", err)
				}

				if tt.check != nil {
					tt.check(t, out)
				}

				return
			}

			if err == nil || err.Error() != tt.wantErrMsg {
				t.Fatalf("Execute() error = %v, want %q", err, tt.wantErrMsg)
			}

			if tt.wantUnauthorized {
				var unauthorized *domainerror.UnauthorizedError
				if !errors.As(err, &unauthorized) {
					t.Fatalf("Execute() error = %v, want *domainerror.UnauthorizedError", err)
				}
			}

			if out != (Output{}) {
				t.Fatalf("Execute() output = %+v, want zero value", out)
			}
		})
	}
}
