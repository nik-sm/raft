all: dirs
	RAFT_DURATION=20 RAFT_VERBOSE=true docker-compose up --build --remove-orphans

test: dirs
	RAFT_DURATION=60 RAFT_VERBOSE=true docker-compose up --build --remove-orphans &
	sleep 10 
	docker network disconnect --force raft-network r0
	sleep 10
	docker network connect raft-network r0


dirs:
	mkdir -p raft_node_persistence

clean:
	rm -rf raft_node_persistence
