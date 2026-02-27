import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const processingTime = new Trend('batch_processing_time_ms');

export let options = {
    scenarios: {
        batch_stress: {
            executor: 'ramping-vus',
            startVUs: 1,
            stages: [
                { duration: '30s', target: 10 },
                { duration: '1m', target: 30 },
                { duration: '30s', target: 50 },
                { duration: '30s', target: 0 },
            ],
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<2000', 'p(99)<5000'],
        http_req_failed: ['rate<0.05'],
        errors: ['rate<0.1'],
    },
};

export default function () {
    const page = Math.floor(Math.random() * 3) + 1;
    const limit = [5, 10, 20, 50][Math.floor(Math.random() * 4)];

    const res = http.get(
        `http://localhost:8080/recommendations/batch?page=${page}&limit=${limit}`
    );

    const success = check(res, {
        'status is 200': (r) => r.status === 200,
        'has results array': (r) => {
            try {
                const body = JSON.parse(r.body);
                return Array.isArray(body.results);
            } catch {
                return false;
            }
        },
        'has summary': (r) => {
            try {
                const body = JSON.parse(r.body);
                return body.summary && body.summary.success_count !== undefined;
            } catch {
                return false;
            }
        },
        'has pagination info': (r) => {
            try {
                const body = JSON.parse(r.body);
                return body.page !== undefined && body.limit !== undefined && body.total_users !== undefined;
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
            processingTime.add(body.summary.processing_time_ms);
        } catch {}
    }

    sleep(0.5);
}
