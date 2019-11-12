all:
	PRJ2_TEST_CASE=1 PRJ2_DURATION=60 PRJ2_VERBOSE=true docker-compose up --build --remove-orphans
