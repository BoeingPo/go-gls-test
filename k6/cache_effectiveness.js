import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';

const cacheHits = new Counter('cache_hits');
const cacheMisses = new Counter('cache_misses');
const cacheHitRate = new Rate('cache_hit_rate');

export let options = {
    scenarios: {
        cache_effectiveness: {
            executor: 'ramping-arrival-rate',
            startRate: 10,
            timeUnit: '1s',
            preAllocatedVUs: 20,
            maxVUs: 100,
            stages: [
                { duration: '30s', target: 50 },
                { duration: '1m', target: 100 },
                { duration: '30s', target: 0 },
            ],
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<500'],
        http_req_failed: ['rate<0.01'],
    },
};

// Use a small set of user IDs to maximize cache hits
const USER_IDS = [1, 2, 3, 4, 5];
const LIMIT = 10;

export default function () {
    // Repeatedly request the same users to test cache effectiveness
    const userId = USER_IDS[Math.floor(Math.random() * USER_IDS.length)];

    const res = http.get(
        `http://localhost:8080/users/${userId}/recommendations?limit=${LIMIT}`
    );

    const isOk = check(res, {
        'status is 200': (r) => r.status === 200,
    });

    if (isOk) {
        try {
            const body = JSON.parse(res.body);
            if (body.metadata && body.metadata.cache_hit === true) {
                cacheHits.add(1);
                cacheHitRate.add(1);
            } else {
                cacheMisses.add(1);
                cacheHitRate.add(0);
            }
        } catch {}
    }

    sleep(0.05);
}
