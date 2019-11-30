all:
	RAFT_DURATION=20 RAFT_VERBOSE=true docker-compose up --build --remove-orphans
