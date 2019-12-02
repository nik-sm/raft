all: dirs
	RAFT_DURATION=20 RAFT_VERBOSE=false docker-compose up --build --remove-orphans

test_stop1: dirs
	RAFT_DURATION=60 RAFT_VERBOSE=true docker-compose up --build --remove-orphans &
	sleep 20
	docker stop r0

test_stop2: dirs
	RAFT_DURATION=60 RAFT_VERBOSE=true docker-compose up --build --remove-orphans &
	sleep 20
	docker stop r0
	docker stop r1

test_disconnect1: dirs
	RAFT_DURATION=60 RAFT_VERBOSE=true docker-compose up --build --remove-orphans &
	sleep 20
	docker network disconnect --force raft-network r0

test_disconnect2: dirs
	RAFT_DURATION=60 RAFT_VERBOSE=true docker-compose up --build --remove-orphans &
	sleep 20
	docker network disconnect --force raft-network r0
	docker network disconnect --force raft-network r1

test_disconnect_reconnect: dirs
	RAFT_DURATION=60 RAFT_VERBOSE=true docker-compose up --build --remove-orphans &
	sleep 20
	docker network disconnect --force raft-network r0
	sleep 20
	docker network connect raft-network r0


dirs:
	mkdir -p persistence

clean:
	rm -rf persistence
