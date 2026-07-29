-- Mixed read/write workload for the demo users and billing services.
local requests = {
  wrk.format("GET", "/api/hello"),
  wrk.format("GET", "/api/users"),
  wrk.format("GET", "/api/users/42"),
  wrk.format("GET", "/api/billing/invoices"),
  wrk.format("GET", "/api/billing/invoices/inv-1002"),
  wrk.format("POST", "/api/billing/payments", {
    ["Content-Type"] = "application/json"
  }, "{}")
}

local index = 0
local statuses = {}

request = function()
  index = (index % #requests) + 1
  return requests[index]
end

response = function(status)
  statuses[status] = (statuses[status] or 0) + 1
end

done = function()
  io.write("\nHTTP status counts:\n")
  for status, count in pairs(statuses) do
    io.write(string.format("  %d: %d\n", status, count))
  end
end
