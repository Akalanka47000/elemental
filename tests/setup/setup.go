package e_test_setup

import (
	"context"
	"elemental/connection"
	"elemental/tests/base"
	"elemental/tests/mocks"
)

func Connection() {
	e_connection.ConnectURI(e_mocks.DB_URI)
}

func Seed() {
	e_test_base.UserModel.InsertMany(e_mocks.Users)
}

func SeededConnection() {
	Connection()
	Seed()
}

func Teardown() {
	e_connection.UseDefault().Drop(context.TODO())
	e_connection.Use(e_mocks.SECONDARY_DB).Drop(context.TODO())
	e_connection.Disconnect()
}
