package elemental

import (
	"context"
	"github.com/elcengine/elemental/utils"
	"errors"

	"github.com/samber/lo"
	"github.com/spf13/cast"
)

func (m Model[T]) Where(field string, equals ...any) Model[T] {
	m.whereField = field
	if len(equals) > 0 {
		m = m.Equals(equals[0])
	}
	return m
}

func (m Model[T]) OrWhere(field string, equals ...any) Model[T] {
	m.whereField = field
	m.orConditionActive = true
	if len(equals) > 0 {
		m = m.Equals(equals[0])
	}
	return m
}

func (m Model[T]) OrFail(err ...error) Model[T] {
	if len(err) > 0 {
		m.failWith = &err[0]
	} else {
		m.failWith = lo.ToPtr(errors.New("no results found matching the given query"))
	}
	return m
}

func (m Model[T]) Exec(ctx ...context.Context) any {
	if m.executor == nil {
		m.executor = func(m Model[T], ctx context.Context) any {
			var results []T
			e_utils.Must(lo.Must(m.Collection().Aggregate(ctx, m.pipeline)).All(ctx, &results))
			m.checkConditionsAndPanic(results)
			return results
		}
	}
	if m.schedule != nil {
		id, err := cron.AddFunc(*m.schedule, func() {
			m.executor(m, e_utils.DefaultCTX(ctx))
		})
		if err != nil {
			panic(errors.New("failed to schedule query"))
		}
		cron.Start()
		return cast.ToInt(id)
	}
	return m.executor(m, e_utils.DefaultCTX(ctx))
}
