# JSON Pointer  

| Format         | Syntax Example       | Purpose                            | Reference                                                                                            |
|----------------|----------------------|------------------------------------|------------------------------------------------------------------------------------------------------|
| JSON Pointer   | `/myobj/3/lamp`      | Canonical path referencing in JSON | [RFC 6901](https://datatracker.ietf.org/doc/html/rfc6901)                                            |
| JSONPath       | `$.myobj[3].lamp`    | Query-like access to JSON data     | [Stefan Goessner](https://goessner.net/articles/JsonPath/)                                           |
| Dot Notation   | `myobj[3].lamp`      | Informal, human-readable reference | Informal usage in docs & tools                                                                       |
| JavaScript Expr| `root.myobj[3].lamp` | Used in JS/DOM for property access | [MDN JavaScript](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/Working_with_Objects) |
