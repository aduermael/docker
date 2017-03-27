--
-- images test
--

Tests = { id = "1" }

function Tests:run()
  print("RUNNING tests...")
  print(self.id)
end

local tests = Tests
tests.id = "2"

return tests
