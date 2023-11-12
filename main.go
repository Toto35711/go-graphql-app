package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Book struct {
	ID     primitive.ObjectID `json:"id" bson:"_id"`
	Title  string             `json:"title" bson:"title"`
	Author string             `json:"author" bson:"author"`
}

var rootQuery = graphql.NewObject(
	graphql.ObjectConfig{
		Name: "RootQuery",
		Fields: graphql.Fields{
			"book": &graphql.Field{
				Type: graphql.NewObject(
					graphql.ObjectConfig{
						Name: "Book",
						Fields: graphql.Fields{
							"id":     &graphql.Field{Type: graphql.String},
							"title":  &graphql.Field{Type: graphql.String},
							"author": &graphql.Field{Type: graphql.String},
						},
					},
				),
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					id, ok := p.Args["id"].(string)
					if ok {
						filter := bson.M{"_id": id}
						var result Book
						err := booksCollection.FindOne(context.Background(), filter).Decode(&result)
						if err != nil {
							log.Printf("Error finding book by ID: %v", err)
							return nil, err
						}
						return result, nil
					}
					return nil, nil
				},
			},
			"books": &graphql.Field{
				Type: graphql.NewList(
					graphql.NewObject(
						graphql.ObjectConfig{
							Name: "Book",
							Fields: graphql.Fields{
								"id":     &graphql.Field{Type: graphql.String},
								"title":  &graphql.Field{Type: graphql.String},
								"author": &graphql.Field{Type: graphql.String},
							},
						},
					),
				),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					cursor, err := booksCollection.Find(context.Background(), bson.M{})
					if err != nil {
						log.Printf("Error finding books: %v", err)
						return nil, err
					}
					defer cursor.Close(context.Background())

					var results []Book
					if err := cursor.All(context.Background(), &results); err != nil {
						log.Printf("Error decoding books: %v", err)
						return nil, err
					}
					return results, nil
				},
			},
		},
	},
)

var bookType = graphql.NewObject(
	graphql.ObjectConfig{
		Name: "Book",
		Fields: graphql.Fields{
			"id":     &graphql.Field{Type: graphql.String},
			"title":  &graphql.Field{Type: graphql.String},
			"author": &graphql.Field{Type: graphql.String},
		},
	},
)

var bookInputType = graphql.NewInputObject(
	graphql.InputObjectConfig{
		Name: "BookInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"title":  &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"author": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		},
	},
)

var mutation = graphql.NewObject(
	graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"createBook": &graphql.Field{
				Type: bookType,
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{
						Type: bookInputType,
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					input, ok := p.Args["input"].(map[string]interface{})
					if !ok {
						return nil, errors.New("invalid input format")
					}

					newBook := Book{
						ID:     primitive.NewObjectID(),
						Title:  input["title"].(string),
						Author: input["author"].(string),
					}

					result, err := booksCollection.InsertOne(context.Background(), newBook)
					if err != nil {
						log.Printf("Error creating a new book: %v", err)
						return nil, err
					}

					newBook.ID = result.InsertedID.(primitive.ObjectID)

					return newBook, nil
				},
			},
		},
	},
)

var schema, _ = graphql.NewSchema(
	graphql.SchemaConfig{
		Query:    rootQuery,
		Mutation: mutation,
	},
)

var booksCollection *mongo.Collection

func init() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	mongoURI := os.Getenv("MONGOURI")
	if mongoURI == "" {
		log.Fatal("MONGOURI is not set in .env file")
	}

	clientOptions := options.Client().ApplyURI(mongoURI)

	client, err := mongo.NewClient(clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = client.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}

	booksCollection = client.Database("graphql").Collection("books")
}

func graphqlHandler(w http.ResponseWriter, r *http.Request) {
	var requestBody map[string]interface{}

	err := json.NewDecoder(r.Body).Decode(&requestBody)
	if err != nil {
		http.Error(w, "Error decoding request body", http.StatusBadRequest)
		return
	}

	query, exists := requestBody["query"].(string)
	if !exists || query == "" {
		http.Error(w, "Must provide a GraphQL query", http.StatusBadRequest)
		return
	}

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: query,
	})

	json.NewEncoder(w).Encode(result)
}

func main() {
	http.HandleFunc("/graphql", graphqlHandler)
	fmt.Println("Server is running on http://localhost:8080/graphql")
	log.Fatal(http.ListenAndServe(":8070", nil))
}
