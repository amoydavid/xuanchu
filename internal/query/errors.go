package query

import "errors"

var ErrProjectPredicateUnresolved = errors.New("project predicate must be resolved before SQL compilation")
