## Changelog

#### Update 10 September 2016

- Add support for go1.7 net.Context

#### Update 25 September 2015

- Add support for Sub router

Example :
``` go
func main() {
    mux := bone.New()
    sub := mux.NewRouter()

    sub.GetFunc("/test/example", func(rw http.ResponseWriter, req *http.Request) {
        rw.Write([]byte("From sub router !"))
    })

    mux.SubRoute("/api", sub)

    http.ListenAndServe(":8080", mux)
}

```


#### Update 26 April 2015

- Add Support for REGEX parameters, using ` # ` instead of ` : `.
- Add Mux method ` mux.GetFunc(), mux.PostFunc(), etc ... `, takes ` http.HandlerFunc ` instead of ` http.Handler `.

Example :
``` go
func main() {
    mux.GetFunc("/route/#var^[a-z]$", handler)
}

func handler(rw http.ResponseWriter, req *http.Request) {
    bone.GetValue(req, "var")
}
```

#### Update 29 january 2015

- Speed improvement for url Parameters, from ```~ 1500 ns/op ``` to ```~ 1000 ns/op ```.

#### Update 25 december 2014

After trying to find a way of using the default url.Query() for route parameters, i decide to change the way bone is dealing with this. url.Query() is too slow for good router performance.
So now to get the parameters value in your handler, you need to use
` bone.GetValue(req, key) ` instead of ` req.Url.Query().Get(key) `.
This change give a big speed improvement for every kind of application using route parameters, like ~80x faster ...
Really sorry for breaking things, but i think it's worth it.

