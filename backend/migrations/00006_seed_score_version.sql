-- +goose Up
-- Baseline GCI weights and display thresholds from doc bab 12C-12E.
-- These are a product starting point, not universal truth: recalibrate against
-- user corrections and publish a new score_version rather than editing this row.

INSERT INTO score_versions (version, formula_json, thresholds, notes) VALUES (
    'gci-1.0',
    '{
      "gci_weights": {
        "emotional_climate": 0.20,
        "support_warmth": 0.15,
        "resolution": 0.15,
        "communication_balance": 0.10,
        "responsiveness": 0.10,
        "de_escalation": 0.15,
        "issue_closure": 0.15
      },
      "inverted_components": ["de_escalation", "issue_closure"],
      "reliability_weights": {
        "coverage": 0.25,
        "average_confidence": 0.25,
        "context_sufficiency": 0.20,
        "modality_quality": 0.15,
        "sample_sufficiency": 0.15
      },
      "context_weight": { "low": 0.40, "medium": 0.70, "high": 1.00 }
    }'::jsonb,
    '{
      "reliability_status": {
        "insufficient": [0, 39],
        "provisional":  [40, 59],
        "sufficient":   [60, 74],
        "high":         [75, 89],
        "very_high":    [90, 100]
      },
      "gci_labels": {
        "tekanan_komunikasi_sangat_tinggi": [0, 20],
        "banyak_hambatan_komunikasi":       [21, 40],
        "campuran_atau_tidak_stabil":       [41, 60],
        "relatif_konstruktif":              [61, 80],
        "konsisten_konstruktif":            [81, 100]
      },
      "minimum_data": {
        "min_eligible_messages": 50,
        "min_sessions": 3,
        "min_active_days": 2,
        "min_coverage": 0.70,
        "min_average_confidence": 0.60,
        "min_context_medium_or_high_ratio": 0.60
      },
      "period_comparison": {
        "coverage_warning_delta_points": 15,
        "min_reliability_for_conclusion": 60
      }
    }'::jsonb,
    'Baseline dari dokumen konsep v1.3 bab 12C-12E. Wajib dikalibrasi ulang terhadap koreksi pengguna.'
);

-- +goose Down
DELETE FROM score_versions WHERE version = 'gci-1.0';
