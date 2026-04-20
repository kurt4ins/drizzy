#!/usr/bin/env bash
set -euo pipefail

DURATION=30s
CONSUMERS=4
PRODUCERS=4
RESULTS_FILE="report/results.csv"

mkdir -p report

# CSV header
echo "broker,size_bytes,rate,sent,received,lost,errors,throughput,p50,p95,p99,max" > "$RESULTS_FILE"

# Build both binaries once
go build -o bin/producer ./cmd/producer
go build -o bin/consumer ./cmd/consumer

run_experiment() {
    local broker=$1
    local size=$2
    local rate=$3

    echo ""
    echo ">>> broker=$broker  size=${size}B  rate=$rate msg/s"

    # Flush leftover messages from previous run
    if [ "$broker" = "rabbitmq" ]; then
        docker compose exec -T rabbitmq rabbitmqadmin purge queue name=bench 2>/dev/null || true
    else
        # Restart Redis to recover from any prior OOM crash, then wait for it to be ready
        docker compose restart redis
        until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do
            sleep 1
        done
        docker compose exec -T redis redis-cli DEL bench 2>/dev/null || true
    fi

    # Start consumers in background, capture output
    ./bin/consumer -broker "$broker" -consumers "$CONSUMERS" -duration "$DURATION" \
        > /tmp/consumer_out.txt 2>&1 &
    CONSUMER_PID=$!

    # Give consumer time to connect and register
    sleep 2

    # Run producer (blocks until duration elapses)
    PRODUCER_OUT=$(./bin/producer \
        -broker "$broker" \
        -size "$size" \
        -rate "$rate" \
        -duration "$DURATION" \
        -producers "$PRODUCERS")

    echo "  PRODUCER: $PRODUCER_OUT"

    # Wait for consumer to drain remaining messages (extra 5s buffer)
    sleep 5
    kill "$CONSUMER_PID" 2>/dev/null || true
    wait "$CONSUMER_PID" 2>/dev/null || true

    CONSUMER_OUT=$(cat /tmp/consumer_out.txt)
    echo "  CONSUMER: $CONSUMER_OUT"

    # Append producer line to CSV (it has all the fields we need)
    echo "$PRODUCER_OUT" | awk -v b="$broker" -v s="$size" -v r="$rate" '
    {
        # parse key=value pairs into CSV
        n=split($0, a, "  *")
        row=b "," s "," r
        for (i=1; i<=n; i++) {
            split(a[i], kv, "=")
            row=row "," kv[2]
        }
        # only print the numeric fields (skip broker/size/rate which we already have)
        print row
    }' >> "$RESULTS_FILE" || echo "$broker,$size,$rate,$PRODUCER_OUT" >> "$RESULTS_FILE"
}

# ── Experiment 1: Baseline ────────────────────────────────────────────────────
echo "=== EXPERIMENT 1: BASELINE (size=1024B, rate=5000) ==="
for broker in rabbitmq redis; do
    run_experiment "$broker" 1024 5000
done

# ── Experiment 2: Message size sweep ─────────────────────────────────────────
echo ""
echo "=== EXPERIMENT 2: MESSAGE SIZE SWEEP (rate=5000) ==="
for broker in rabbitmq redis; do
    for size in 128 1024 10240 102400; do
        run_experiment "$broker" "$size" 5000
    done
done

# ── Experiment 3: Rate sweep to saturation ───────────────────────────────────
echo ""
echo "=== EXPERIMENT 3: RATE SWEEP TO SATURATION (size=1024B) ==="
for broker in rabbitmq redis; do
    for rate in 1000 5000 10000 20000 50000; do
        run_experiment "$broker" 1024 "$rate"
        # If producer reported lost > 0, broker is saturated — stop increasing rate
        if grep -q "lost=[1-9]" /tmp/consumer_out.txt 2>/dev/null; then
            echo "  !! Saturation detected for $broker at $rate msg/s — stopping sweep"
            break
        fi
    done
done

echo ""
echo "=== DONE. Results written to $RESULTS_FILE ==="