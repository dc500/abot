// Package search finds items from Ava's repository of knowledge.
package search

import (
	"encoding/json"
	"os"

	"github.com/avabot/ava/Godeps/_workspace/src/github.com/mattbaird/elastigo/lib"
	"github.com/avabot/ava/shared/datatypes"
)

type ElasticClient struct {
	*elastigo.Conn
}

type Bucket struct {
	Key      string
	DocCount uint `json:"doc_count"`
}

func NewClient() *ElasticClient {
	client := elastigo.NewConn()
	client.Username = os.Getenv("ELASTICSEARCH_USERNAME")
	client.Password = os.Getenv("ELASTICSEARCH_PASSWORD")
	client.Domain = os.Getenv("ELASTICSEARCH_DOMAIN")
	return &ElasticClient{client}
}

func (ec *ElasticClient) FindProducts(query, typ string, count int) (
	[]datatypes.Product, error) {
	q := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]string{"_all": query},
		},
	}
	res, err := ec.Search("products", typ, nil, q)
	if err != nil {
		return []datatypes.Product{}, err
	}
	if res.Hits.Total == 0 {
		return []datatypes.Product{}, nil
	}
	var products []datatypes.Product
	for _, hit := range res.Hits.Hits {
		var prod struct {
			Name  string
			Price uint64
		}
		err = json.Unmarshal([]byte(*hit.Source), &prod)
		if err != nil {
			return products, err
		}
		product := datatypes.Product{
			ID:    hit.Id,
			Name:  prod.Name,
			Price: prod.Price,
		}
		products = append(products, product)
	}
	return products, nil
}

func (ec *ElasticClient) FindProductKeywords(typ string) ([]Bucket, error) {
	q := map[string]interface{}{
		"aggs": map[string]interface{}{
			"keywords": map[string]interface{}{
				"terms": map[string]interface{}{
					"field":         "Reviews.Body",
					"size":          2500,
					"min_doc_count": 3,
				},
			},
		},
	}
	res, err := ec.Search("products", typ, nil, q)
	if err != nil {
		return []Bucket{}, err
	}
	var aggs struct {
		Keywords struct {
			Buckets []Bucket
		}
	}
	err = json.Unmarshal([]byte(res.Aggregations), &aggs)
	if err != nil {
		return []Bucket{}, err
	}
	return aggs.Keywords.Buckets, nil
}
