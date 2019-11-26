all:
	RAFT_DURATION=60 RAFT_VERBOSE=true docker-compose up --build --remove-orphans
