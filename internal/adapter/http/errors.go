package http

import (
	"errors"
	"net/http"

	domainerror "wherewhat/internal/domain/error"
)

// writeUseCaseError maps a use case error onto an HTTP response: the use case builds the exact
// user-facing message, this only picks the status code from the error's type. An error of no known
// domain type is an internal failure and becomes a 500.
func writeUseCaseError(w http.ResponseWriter, err error) {
	var verr *domainerror.ValidationError
	if errors.As(err, &verr) {
		writeError(w, http.StatusBadRequest, verr.Error())
		return
	}

	var nferr *domainerror.NotFoundError
	if errors.As(err, &nferr) {
		writeError(w, http.StatusNotFound, nferr.Error())
		return
	}

	var cerr *domainerror.ConflictError
	if errors.As(err, &cerr) {
		writeError(w, http.StatusConflict, cerr.Error())
		return
	}

	var ferr *domainerror.ForbiddenError
	if errors.As(err, &ferr) {
		writeError(w, http.StatusForbidden, ferr.Error())
		return
	}

	var uerr *domainerror.UnauthorizedError
	if errors.As(err, &uerr) {
		writeError(w, http.StatusUnauthorized, uerr.Error())
		return
	}

	writeError(w, http.StatusInternalServerError, err.Error())
}
