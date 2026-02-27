import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const recommendationCount = new Trend('recommendation_count');

export let options = {
    scenarios: {
        single_user_load: {
            executor: 'constant-arrival-rate',
            rate: 100,
            timeUnit: '1s',
            duration: '1m',
            preAllocatedVUs: 50,
            maxVUs: 200,
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<500', 'p(99)<1000'],
        http_req_failed: ['rate<0.01'],
        errors: ['rate<0.05'],
    },
};

export default function () {
    const userId = Math.floor(Math.random() * 30) + 1;
    const limit = Math.floor(Math.random() * 10) + 5;

    const res = http.get(
        `http://localhost:8080/users/${userId}/recommendations?limit=${limit}`
    );

    const success = check(res, {
        'status is 200': (r) => r.status === 200,
        'has recommendations': (r) => {
            try {
                const body = JSON.parse(r.body);
                return body.recommendations && body.recommendations.length > 0;
            } catch {
                return false;
            }
        },
        'has metadata': (r) => {
            try {
                const body = JSON.parse(r.body);
                return body.metadata && body.metadata.generated_at !== undefined;
            } catch {
                return false;
            }
        },
        'has valid user_id': (r) => {
            try {
                const body = JSON.parse(r.body);
                return body.user_id === userId;
            } catch {
                return false;
            }
        },
    });

    if (!success) {
        errorRate.add(1);
    } else {
        errorRate.add(0);
        try {
            const body = JSON.parse(res.body);
            recommendationCount.add(body.recommendations.length);
        } catch {}
    }

    sleep(0.01);
}
