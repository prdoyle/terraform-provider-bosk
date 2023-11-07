resource "object_node" "example" {
  url        = "http://localhost:1740/bosk/"
  value_json = "[{\"world\":{\"id\":\"world\"}}]"
}
