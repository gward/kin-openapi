package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

func main() {
	ctx := context.Background()

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("simplespec.yml")
	//fmt.Printf("doc = %T %+v\nerr = %v\n", doc, doc, err)
	if err != nil {
		log.Fatal(err.Error())
	}

// 	body := bytes.NewBufferString(`{
//   "name": "bob",
//   "length": -1
// }`)

// 	reqBuf := bytes.NewBufferString(`POST https://api.example.com/v0/foo/aab?bip=3 HTTP/1.1
// Host: api.example.com
// Content-Type: application/json

// {
//   "name": "bob",
//   "length": -1
// }
// `)
//	req, err := http.ReadRequest(bufio.NewReader(reqBuf))

	type testRequest struct {
		desc   string
		method string
		path   string
		body   string
	}

	testRequests := []testRequest{
		{
			"valid GET request",
			"GET",
			"/v0/foo",
			"",
		},
		{
			"valid GET request with query param",
			"GET",
			"/v0/foo?bip=38",
			"",
		},
		{
			"invalid GET request: query param empty",
			"GET",
			"/v0/foo?bip=",
			"",
		},
		{
			"invalid GET request: query param not integer",
			"GET",
			"/v0/foo?bip=4x",
			"",
		},
		{
			"invalid GET request: query param below minimum",
			"GET",
			"/v0/foo?bip=0",
			"",
		},
		{
			"valid POST request",
			"POST",
			"/v0/foo/aab",
			`{
				"name": "bob",
				"length": 5
			}`,
		},
		{
			"invalid POST request: 1 error, in the body",
			"POST",
			"/v0/foo/bab?bip=3",
			`{
				"name": "bob",
				"length": -1
			}`,
		},
		{
			"very invalid POST request: bad query parma, bad path param, bad body",
			"POST",
			"/v0/foo/bab?bip=4x",
			`{
				"name": "a",
				"length": 3,
				"pizza": {
					"toppings": ["MUSHROOM", "PINEAPPLE"]
				}
			}`,
		},
	}


	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		log.Fatal(err.Error())
	}

	for i, testReq := range testRequests {
		method := testReq.method
		var bodyBuf *bytes.Buffer
		var ctypes []string
		if method == "POST" || method == "PUT" {
			bodyBuf = bytes.NewBufferString(testReq.body)
			ctypes = append(ctypes, "application/json")
		} else {
			bodyBuf = bytes.NewBuffer(nil)
		}

		req, err := http.NewRequest(
			testReq.method,
			"https://api.example.com" + testReq.path,
			bodyBuf)
		fmt.Printf(
			"test req %d: %s\n%s %s (%d byte body)\n",
			i, testReq.desc, req.Method, req.URL, len(testReq.body))
		if err != nil {
			log.Fatal(err.Error())
		}
		if ctypes != nil {
			req.Header["Content-Type"] = ctypes
		}

		validateRequest(ctx, router, req)
		fmt.Println()
	}

}

func validateRequest(ctx context.Context, router routers.Router, req *http.Request) {
	route, pathParams, err := router.FindRoute(req)
	//fmt.Printf("pathParams = %v\n", pathParams)
	if err != nil {
		log.Fatal(err.Error())
	}

	valInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			MultiError: true,
		},
	}
	err = openapi3filter.ValidateRequest(ctx, valInput)
	if err == nil {
		fmt.Printf("no validation errors\n")
		return
	}
	dumpKinError(err, os.Stdout, "")


	// issues := convertMultiError(err.(openapi3.MultiError))
	// fmt.Printf("request validation err = %T (%d issues)\n", err, len(issues))

	// for key, val := range issues {
	// 	fmt.Printf("%s: %T %v\n", key, val, val)
	// }

}

const (
	prefixBody = "@body"
	unknown    = "@unknown"
)

func dumpKinError(err error, writer io.Writer, indent string) {
	switch err := err.(type) {
	case openapi3.MultiError:
		fmt.Fprintf(
			writer,
			"%s%T (len %d)\n",
			indent,
			err,
			len(err))
		for _, innerErr := range err {
			dumpKinError(innerErr, writer, "  "+indent)
		}
	case *openapi3filter.RequestError:
		fmt.Fprintf(
			writer,
			"%s%T: {Input: %T, Parameter: %T, Reason: %q}\n",
			indent,
			err,
			err.Input,
			err.Parameter,
			err.Reason)
		dumpKinError(err.Err, writer, "  "+indent)
	case *openapi3.SchemaError:
		fmt.Fprintf(
			writer,
			"%s%T: {Value: %T %v, Schema: %T, SchemaField: %q, Reason: %q}\n",
			indent,
			err,
			err.Value,
			err.Value,
			err.Schema,
			err.SchemaField,
			err.Reason)
		if err.Origin != nil {
			dumpKinError(err.Origin, writer, "  "+indent)
		}
	default:
		fmt.Fprintf(writer, "%s%T: %s\n", indent, err, err)
	}
}

func convertMultiError(err openapi3.MultiError) map[string][]string {
	issues := make(map[string][]string)
	for i, err := range err {
		fmt.Printf("err[%d] = %T\n", i, err)
		switch err := err.(type) {
		case *openapi3.SchemaError:
			fmt.Printf(
				"found %T error: {\n" +
					"  Value: %T %v\n" +
					"  SchemaField: %q\n" +
					"  Reason: %q\n" +
					"  Origin: %T %v\n" +
					"  JSONPointer(): %v\n" +
					"}\n",
				err,
				err.Value, err.Value,
				err.SchemaField,
				err.Reason,
				err.Origin, err.Origin,
				err.JSONPointer())

			field := prefixBody
			if path := err.JSONPointer(); len(path) > 0 {
				field = fmt.Sprintf("%s.%s", field, strings.Join(path, "."))
			}
			issues[field] = append(issues[field], err.Error())

		case *openapi3filter.RequestError:
			var msg string
			// case err := err.Err.(type) {
			
			// }
			if merr, ok := err.Err.(openapi3.MultiError); ok {
				msg = fmt.Sprintf("%d sub-errors", len(merr))
			} else {
				msg = err.Err.Error()
			}

			fmt.Printf(
				"found %T error: {\n" +
					"  Parameter: %v\n" +
					"  RequestBody: %v\n" +
					"  Reason: %q\n" +
					"  Err: %T: %s\n" +
					"}\n",
				err,
				err.Parameter,
				err.RequestBody,
				err.Reason,
				err.Err, msg)

			if err, ok := err.Err.(openapi3.MultiError); ok {
				// RequestError wraps a MultiError: does that mean
				// multiple problems with the same field?
				for key, val := range convertMultiError(err) {
					issues[key] = append(issues[key], val...)
				}
				continue
			}

			// Check if invalid HTTP parameter.
			if err.Parameter != nil {
				prefix := err.Parameter.In
				name := fmt.Sprintf("%s.%s", prefix, err.Parameter.Name)
				issues[name] = append(issues[name], err.Error())
				continue
			}

			// Check if request body
			if err.RequestBody != nil {
				issues[prefixBody] = append(issues[prefixBody], err.Error())
				continue
			}

		default:
			issues[unknown] = append(issues[unknown], err.Error())
		}
	}

	return issues
}
