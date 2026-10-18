resource "bosk_node" "world" {
  path = "/targets/world"
  value = jsonencode({
    id   = "world"
    name = "World"
  })
}
