<?php

namespace App\Http\Controllers;

use App\Support\PrometheusMetrics;

class MetricsController extends Controller
{
    public function index()
    {
        return response(PrometheusMetrics::render(), 200, [
            'Content-Type' => 'text/plain; version=0.0.4; charset=utf-8',
        ]);
    }
}
