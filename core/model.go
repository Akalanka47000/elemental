package elemental

import (
	"context"
	"elemental/connection"
	"elemental/constants"
	"elemental/utils"
	"reflect"
	"strings"

	"github.com/gertd/go-pluralize"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
)

type Model[T any] struct {
	Name       string
	Schema     Schema
	pipeline   mongo.Pipeline
	executor   func(ctx context.Context) any
	whereField string
	failWith   *error
}

var Models = make(map[string]Model[any])

func NewModel[T any](name string, schema Schema) Model[T] {
	if _, ok := Models[name]; ok {
		return e_utils.Cast[Model[T]](Models[name])
	}
	schema.Options.Collection, _ = lo.Coalesce(schema.Options.Collection, pluralize.NewClient().Plural(strings.ToLower(name)))
	model := Model[T]{
		Name:   name,
		Schema: schema,
	}
	Models[name] = e_utils.Cast[Model[any]](model)
	connectionReady := func() {
		model.CreateCollection()
		model.SyncIndexes()
	}
	if model.Ping() != nil {
		e_connection.On(event.ConnectionReady, connectionReady)
	} else {
		connectionReady()
	}
	return model
}

func (m Model[T]) Create(doc T, ctx ...context.Context) T {
	document := enforceSchema(m.Schema, &doc)
	lo.Must(m.Collection().InsertOne(e_utils.DefaultCTX(ctx), document))
	return document
}

func (m Model[T]) InsertMany(docs []T, ctx ...context.Context) []T {
	var documents []interface{}
	for _, doc := range docs {
		documents = append(documents, enforceSchema(m.Schema, &doc))
	}
	lo.Must(m.Collection().InsertMany(e_utils.DefaultCTX(ctx), documents))
	return e_utils.CastSlice[T](documents)
}

func (m Model[T]) Find(query ...primitive.M) Model[T] {
	m.pipeline = append(m.pipeline, bson.D{{Key: "$match", Value: e_utils.DefaultQuery(query...)}})
	return m
}

func (m Model[T]) FindOne(query ...primitive.M) Model[T] {
	m.pipeline = append(m.pipeline,
		bson.D{{Key: "$match", Value: e_utils.DefaultQuery(query...)}},
		bson.D{{Key: "$limit", Value: 1}},
	)
	m.executor = func(ctx context.Context) any {
		var results []T
		e_utils.Must(lo.Must(m.Collection().Aggregate(ctx, m.pipeline)).All(ctx, &results))
		m.checkConditionsAndPanic(results)
		if len(results) == 0 {
			return nil
		}
		return results[0]
	}
	return m
}

func (m Model[T]) FindByID(id primitive.ObjectID) Model[T] {
	return m.FindOne(primitive.M{"_id": id})
}

func (m Model[T]) CountDocuments(query ...primitive.M) Model[T] {
	m.pipeline = append(m.pipeline, bson.D{{Key: "$match", Value: e_utils.DefaultQuery(query...)}}, bson.D{{Key: "$count", Value: "count"}})
	m.executor = func(ctx context.Context) any {
		var results []map[string]any
		e_utils.Must(lo.Must(m.Collection().Aggregate(ctx, m.pipeline)).All(ctx, &results))
		return int64(e_utils.Cast[int32](results[0]["count"]))
	}
	return m
}

func (m Model[T]) Distinct(field string, query ...primitive.M) Model[T] {
	m.pipeline = append(m.pipeline, bson.D{{Key: "$match", Value: e_utils.DefaultQuery(query...)}}, bson.D{{Key: "$group", Value: primitive.M{"_id": "$" + field}}})
	m.executor = func(ctx context.Context) any {
		var results []map[string]any
		e_utils.Must(lo.Must(m.Collection().Aggregate(ctx, m.pipeline)).All(ctx, &results))
		var distinct []string
		for _, result := range results {
			distinct = append(distinct, e_utils.Cast[string](result["_id"]))
		}
		return distinct
	}
	return m
}

func (m Model[T]) Sort(args ...any) Model[T] {
	if len(args) == 1 {
		for field, order := range e_utils.Cast[primitive.M](args[0]) {
			m = m.addToPipeline("$sort", field, order)
		}
	} else {
		if (len(args) % 2) != 0 {
			panic(e_constants.ErrMustPairSortArguments)
		}
		for i := 0; i < len(args); i += 2 {
			m = m.addToPipeline("$sort", e_utils.Cast[string](args[i]), args[i+1])
		}
	}
	return m
}

func (m Model[T]) Select(fields ...any) Model[T] {
	var selection []string
	if len(fields) == 1 && reflect.TypeOf(fields[0]).Kind() == reflect.String {
		selection = strings.FieldsFunc(fields[0].(string), func(r rune) bool {
			return r == ',' || r == ' '
		})
	} else if len(fields) > 1 {
		selection = e_utils.CastSlice[string](fields)
	} else if reflect.TypeOf(fields[0]).Kind() == reflect.Slice {
		selection = fields[0].([]string)
	}
	if len(selection) > 0 {
		for _, field := range selection {
			if strings.HasPrefix(field, "-") {
				m = m.addToPipeline("$project", field[1:], 0)
			} else {
				m = m.addToPipeline("$project", field, 1)
			}
		}
	} else if reflect.TypeOf(fields[0]).Kind() == reflect.Map {
		for field, value := range e_utils.Cast[primitive.M](fields[0]) {
			m = m.addToPipeline("$project", field, value)
		}
	}
	return m
}