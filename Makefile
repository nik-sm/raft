# Notice that we use `--remove-orphans` as a docker-compose argument. If we try once with 5 nodes, and then reduce to using only 3 nodes, the 2 unused nodes may otherwise be left running. 

all:
	PRJ2_TEST_CASE=1 PRJ2_DURATION=60 PRJ2_VERBOSE=false PRJ2_TFROM=9 PRJ2_TTIL=9 docker-compose up --build --remove-orphans

test1:
	PRJ2_TEST_CASE=1 PRJ2_DURATION=60 PRJ2_VERBOSE=false PRJ2_TFROM=9 PRJ2_TTIL=9 docker-compose up --build --remove-orphans

test2:
	PRJ2_TEST_CASE=2 PRJ2_DURATION=60 PRJ2_VERBOSE=false PRJ2_TFROM=9 PRJ2_TTIL=9 docker-compose up --build --remove-orphans

test3:
	PRJ2_TEST_CASE=3 PRJ2_DURATION=60 PRJ2_VERBOSE=false PRJ2_TFROM=9 PRJ2_TTIL=9 docker-compose up --build --remove-orphans

test4:
	PRJ2_TEST_CASE=4 PRJ2_DURATION=60 PRJ2_VERBOSE=false PRJ2_TFROM=9 PRJ2_TTIL=9 docker-compose up --build --remove-orphans

test5:
	PRJ2_TEST_CASE=5 PRJ2_DURATION=60 PRJ2_VERBOSE=false PRJ2_TFROM=9 PRJ2_TTIL=9 docker-compose up --build --remove-orphans
