all: dirs
	RAFT_DURATION=20 RAFT_VERBOSE=true docker-compose up --build --remove-orphans

test1: dirs
	RAFT_DURATION=60 RAFT_VERBOSE=true docker-compose up --build --remove-orphans &
	sleep 20
	docker network disconnect --force raft-network r0

test2: dirs
	RAFT_DURATION=60 RAFT_VERBOSE=true docker-compose up --build --remove-orphans &
	sleep 20
	docker network disconnect --force raft-network r0
	sleep 20
	docker network connect raft-network r0

test3: dirs
	RAFT_DURATION=60 RAFT_VERBOSE=true docker-compose up --build --remove-orphans &
	sleep 20
	docker network disconnect --force raft-network r0
	sleep 20
	docker network disconnect --force raft-network r1

dirs:
	mkdir -p persistence

clean:
	rm -rf persistence
