#!/bin/bash

# Advanced Projection Features Demo Runner
# This script makes it easy to run different scenarios

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Advanced Projection Features Demo${NC}"
echo

# Check if NATS is running
if ! nc -z localhost 4222 2>/dev/null; then
    echo -e "${YELLOW}⚠️  NATS server not detected on localhost:4222${NC}"
    echo
    echo "Start NATS with JetStream:"
    echo "  docker run -d -p 4222:4222 -p 8222:8222 --name nats-demo nats:latest -js"
    echo
    echo "Or install locally:"
    echo "  nats-server -js"
    echo
    exit 1
fi

echo -e "${GREEN}✅ NATS server detected${NC}"
echo

# Parse scenario
SCENARIO=${1:-basic}

case $SCENARIO in
    basic|rebuild|interrupted|monitor|concurrent)
        echo "Running scenario: $SCENARIO"
        echo
        go run main.go $SCENARIO
        ;;
    clean)
        echo "Cleaning up databases and NATS stream..."
        rm -f demo_eventstore.db demo_projections.db
        if command -v nats &> /dev/null; then
            nats stream rm DEMO_EVENTS -f 2>/dev/null || true
        fi
        echo "✅ Cleaned up"
        ;;
    all)
        echo "Running all scenarios..."
        echo
        for scenario in basic rebuild interrupted monitor concurrent; do
            echo "================================"
            echo "Scenario: $scenario"
            echo "================================"
            go run main.go $scenario
            echo
            sleep 2
            # Cleanup between scenarios
            rm -f demo_eventstore.db demo_projections.db
        done
        ;;
    *)
        echo "Unknown scenario: $SCENARIO"
        echo
        echo "Available scenarios:"
        echo "  basic        - Basic projection with checkpoint tracking"
        echo "  rebuild      - Demonstrate rebuild optimization"
        echo "  interrupted  - Demonstrate interrupted rebuild detection"
        echo "  monitor      - Monitor checkpoint and NATS consumer state"
        echo "  concurrent   - Events arriving during rebuild"
        echo "  clean        - Clean up databases and NATS stream"
        echo "  all          - Run all scenarios"
        echo
        echo "Usage:"
        echo "  ./run.sh [scenario]"
        echo
        echo "Examples:"
        echo "  ./run.sh basic"
        echo "  ./run.sh rebuild"
        echo "  ./run.sh all"
        exit 1
        ;;
esac
