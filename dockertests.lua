--
-- images test
--


function tests()
	print("running tests...")
	test1()
	print("done running tests.")
end


function assert(condition, errorMessage)
	if condition == false then
		error(errorMessage)
	end
end


function test1()

end
