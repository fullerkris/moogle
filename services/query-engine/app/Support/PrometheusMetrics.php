<?php

namespace App\Support;

use Illuminate\Support\Facades\Redis;
use Throwable;

class PrometheusMetrics
{
    private const REDIS_TOTALS_KEY = 'metrics:query-engine:http_requests_total';
    private const REDIS_BUCKETS_KEY = 'metrics:query-engine:http_request_duration_seconds_bucket';
    private const REDIS_SUM_COUNT_KEY = 'metrics:query-engine:http_request_duration_seconds';

    private const BUCKETS = [0.1, 0.2, 0.4, 0.8, 1.2, 2.5, 5.0];

    public static function recordRequest(int $statusCode, float $durationSeconds): void
    {
        try {
            $redis = Redis::connection('default');

            $redis->hIncrBy(self::REDIS_TOTALS_KEY, (string) $statusCode, 1);

            foreach (self::BUCKETS as $bucket) {
                if ($durationSeconds <= $bucket) {
                    $redis->hIncrBy(self::REDIS_BUCKETS_KEY, self::formatBucket($bucket), 1);
                }
            }

            $redis->hIncrBy(self::REDIS_BUCKETS_KEY, '+Inf', 1);
            $redis->hIncrByFloat(self::REDIS_SUM_COUNT_KEY, 'sum', $durationSeconds);
            $redis->hIncrBy(self::REDIS_SUM_COUNT_KEY, 'count', 1);
        } catch (Throwable $e) {
            return;
        }
    }

    public static function render(): string
    {
        $requestTotals = [];
        $durationBuckets = [];
        $durationStats = [];

        try {
            $redis = Redis::connection('default');
            $requestTotals = $redis->hGetAll(self::REDIS_TOTALS_KEY) ?: [];
            $durationBuckets = $redis->hGetAll(self::REDIS_BUCKETS_KEY) ?: [];
            $durationStats = $redis->hGetAll(self::REDIS_SUM_COUNT_KEY) ?: [];
        } catch (Throwable $e) {
            $requestTotals = [];
            $durationBuckets = [];
            $durationStats = [];
        }

        $lines = [
            '# HELP http_requests_total Total HTTP requests processed by query-engine.',
            '# TYPE http_requests_total counter',
        ];

        if (empty($requestTotals)) {
            $lines[] = 'http_requests_total{job="query-engine",status="200"} 0';
        } else {
            uksort($requestTotals, static function ($a, $b) {
                return (int) $a <=> (int) $b;
            });

            foreach ($requestTotals as $statusCode => $count) {
                $lines[] = sprintf(
                    'http_requests_total{job="query-engine",status="%s"} %s',
                    self::escapeLabelValue((string) $statusCode),
                    self::formatNumber($count)
                );
            }
        }

        $lines[] = '# HELP http_request_duration_seconds Request duration histogram for query-engine.';
        $lines[] = '# TYPE http_request_duration_seconds histogram';

        foreach (self::BUCKETS as $bucket) {
            $bucketKey = self::formatBucket($bucket);
            $count = $durationBuckets[$bucketKey] ?? 0;
            $lines[] = sprintf(
                'http_request_duration_seconds_bucket{job="query-engine",le="%s"} %s',
                $bucketKey,
                self::formatNumber($count)
            );
        }

        $infCount = $durationBuckets['+Inf'] ?? 0;
        $lines[] = sprintf(
            'http_request_duration_seconds_bucket{job="query-engine",le="+Inf"} %s',
            self::formatNumber($infCount)
        );

        $sum = $durationStats['sum'] ?? 0;
        $count = $durationStats['count'] ?? 0;

        $lines[] = sprintf('http_request_duration_seconds_sum{job="query-engine"} %s', self::formatNumber($sum));
        $lines[] = sprintf('http_request_duration_seconds_count{job="query-engine"} %s', self::formatNumber($count));

        return implode("\n", $lines)."\n";
    }

    private static function formatBucket(float $bucket): string
    {
        if (fmod($bucket, 1.0) === 0.0) {
            return sprintf('%.0f', $bucket);
        }

        return rtrim(rtrim(sprintf('%.6F', $bucket), '0'), '.');
    }

    private static function escapeLabelValue(string $value): string
    {
        return str_replace(['\\', '"'], ['\\\\', '\\"'], $value);
    }

    private static function formatNumber($value): string
    {
        if (is_int($value)) {
            return (string) $value;
        }

        if (is_float($value)) {
            return rtrim(rtrim(sprintf('%.6F', $value), '0'), '.');
        }

        if (is_numeric($value)) {
            $numeric = (float) $value;
            if ((float) ((int) $numeric) === $numeric) {
                return (string) ((int) $numeric);
            }

            return rtrim(rtrim(sprintf('%.6F', $numeric), '0'), '.');
        }

        return '0';
    }
}
