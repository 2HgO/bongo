package frat

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"github.com/oleiade/reflections"
	"reflect"
	"log"
	"errors"
	// "fmt"
	// "math"
	// "strings"
)


type Config struct {
	ConnectionString string
	Database string
	EncryptionKey string
	EncryptionKeyPerCollection map[string]string
}

type Connection struct {
	Config *Config
	Session *mgo.Session
}



// Create a new connection and run Connect()
func Connect(config *Config) *Connection {
	conn := &Connection{
		Config:config,
	}

	conn.Connect()

	return conn
}

// Connect to the database using the provided config
func (m *Connection) Connect() {
	session, err := mgo.Dial(m.Config.ConnectionString)

	if err != nil {
		panic(err)
	}

	m.Session = session
}

// Convenience for retrieving a collection by name based on the config passed to the Connection
func (m *Connection) Collection(name string) *mgo.Collection {
	return m.Session.DB(m.Config.Database).C(name)
}

func (m *Connection) GetEncryptionKey(collection string) []byte {
	key, has := m.Config.EncryptionKeyPerCollection[collection]

	if has {
		return []byte(key)
	} else {
		return []byte(m.Config.EncryptionKey)
	}

}

// Get the collection name from an arbitrary interface. Returns type name in snake case
func getCollectionName(mod interface{}) (string) {
	return ToSnake(reflect.Indirect(reflect.ValueOf(mod)).Type().Name())
}

// Ensure that a particular interface has an "Id" field. Panic if not
func ensureIdField(mod interface{}) {
	has, _ := reflections.HasField(mod, "Id")
	if !has {
		panic("Failed to save - model must have Id field")
	}
}


// Save a document. Collection name is interpreted from name of struct
func (c *Connection) Save(mod interface{}) (error, []string) {
	defer func() {
		if r := recover(); r != nil {
			log.Fatal("Failed to save:\n", r)
		}
	}()

	// 1) Make sure mod has an Id field
	ensureIdField(mod)

	// 2) If there's no ID, create a new one
	f, err := reflections.GetField(mod, "Id")
	id := f.(bson.ObjectId)

	if err != nil {
		panic(err)
	}

	isNew := false
	
	if !id.Valid() {
		id := bson.NewObjectId()
		err := reflections.SetField(mod, "Id", id)

		if err != nil {
			panic(err)
		}

		isNew = true
	}

	// Validate?
	if _, ok := mod.(interface{Validate()[]string}); ok {
		results := reflect.ValueOf(mod).MethodByName("Validate").Call([]reflect.Value{})
		if errs, ok := results[0].Interface().([]string); ok {
			if len(errs) > 0 {
				err := errors.New("Validation failed")
				return err, errs
			}
		}
	}

	if isNew {
		if hook, ok := mod.(interface{BeforeCreate()}); ok { 
			hook.BeforeCreate() 
		} 
	} else if hook, ok := mod.(interface{BeforeUpdate()}); ok { 
		hook.BeforeUpdate() 
	}

	if hook, ok := mod.(interface{BeforeSave()}); ok { 
		hook.BeforeSave() 
	} 
	
	colname := getCollectionName(mod)
	// 3) Convert the model into a map using the crypt library
	modelMap := EncryptDocument(c.GetEncryptionKey(colname), mod)

	_, err =  c.Collection(colname).UpsertId(modelMap["_id"], modelMap)

	return err, nil
}

// Find a document by ID. Collection name is interpreted from name of struct
func (c *Connection) FindById(id bson.ObjectId, mod interface{}) (error) {
	returnMap := make(map[string]interface{})

	colname := getCollectionName(mod)
	err := c.Collection(colname).FindId(id).One(&returnMap)
	if err != nil {
		return err
	}

	// Decrypt + Marshal into map
	
	DecryptDocument(c.GetEncryptionKey(colname), returnMap, mod)

	if hook, ok := mod.(interface{AfterFind()}); ok { 
		hook.AfterFind() 
	}
	return nil
}

// Delete a document. Collection name is interpreted from name of struct
func (c *Connection) Delete(mod interface{}) error {
	ensureIdField(mod)
	f, err := reflections.GetField(mod, "Id")
	if err != nil {
		return err
	}
	id := f.(bson.ObjectId)
	colname := getCollectionName(mod)

	return c.Collection(colname).Remove(bson.M{"_id": id})
}



