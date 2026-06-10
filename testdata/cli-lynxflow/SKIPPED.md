# Skipped Transcripts

Total skipped: 40

## already a lynxflow test (5)

- `backend_lynxflow_chain`: already a lynxflow test
- `backend_lynxflow_enrich_outlier`: already a lynxflow test
- `backend_lynxflow_group`: already a lynxflow test
- `backend_lynxflow_keep_omit`: already a lynxflow test
- `backend_lynxflow_let`: already a lynxflow test

## error test (non-zero exit code) (2)

- `error_bad_query`: error test (non-zero exit code)
- `error_nonexistent_file`: error test (non-zero exit code)

## skip: glimpse output is non-deterministic due to sampling (2)

- `backend_glimpse`: skip: glimpse output is non-deterministic due to sampling
- `backend_glimpse`: skip: glimpse output is non-deterministic due to sampling

## translation error (1)

- `backend_where_like`: translate.SPL2ToLynxFlow: output failed LynxFlow validation: unknown stage "like" (generated: from main
| where path like "/api/v2/users%"
| stats count())

## unsupported command: *spl2.CompareCommand (2)

- `backend_compare_shift`: translate.SPL2ToLynxFlow: unsupported command: *spl2.CompareCommand
- `backend_compare_shift`: translate.SPL2ToLynxFlow: unsupported command: *spl2.CompareCommand

## unsupported command: *spl2.CorrelateCommand (2)

- `backend_correlate_pearson`: translate.SPL2ToLynxFlow: unsupported command: *spl2.CorrelateCommand
- `backend_correlate_pearson`: translate.SPL2ToLynxFlow: unsupported command: *spl2.CorrelateCommand

## unsupported command: *spl2.OutliersCommand (3)

- `backend_outliers_iqr`: translate.SPL2ToLynxFlow: unsupported command: *spl2.OutliersCommand
- `backend_outliers_zscore`: translate.SPL2ToLynxFlow: unsupported command: *spl2.OutliersCommand
- `backend_outliers_iqr`: translate.SPL2ToLynxFlow: unsupported command: *spl2.OutliersCommand

## unsupported command: *spl2.PatternsCommand (2)

- `backend_patterns_message`: translate.SPL2ToLynxFlow: unsupported command: *spl2.PatternsCommand
- `backend_patterns_message`: translate.SPL2ToLynxFlow: unsupported command: *spl2.PatternsCommand

## unsupported command: *spl2.RollupCommand (2)

- `backend_rollup_multi`: translate.SPL2ToLynxFlow: unsupported command: *spl2.RollupCommand
- `backend_rollup_multi`: translate.SPL2ToLynxFlow: unsupported command: *spl2.RollupCommand

## unsupported command: *spl2.SelectCommand (1)

- `backend_eventstats_pct`: translate.SPL2ToLynxFlow: unsupported command: *spl2.SelectCommand

## unsupported command: *spl2.SessionizeCommand (2)

- `backend_sessionize_user`: translate.SPL2ToLynxFlow: unsupported command: *spl2.SessionizeCommand
- `backend_sessionize_user`: translate.SPL2ToLynxFlow: unsupported command: *spl2.SessionizeCommand

## unsupported command: append (2)

- `backend_append_error_warn`: translate.SPL2ToLynxFlow: unsupported command: append (use union in LynxFlow)
- `cross_append_errors`: translate.SPL2ToLynxFlow: unsupported command: append (use union in LynxFlow)

## unsupported command: multisearch (5)

- `backend_multisearch_mixed`: translate.SPL2ToLynxFlow: unsupported command: multisearch (use union in LynxFlow)
- `backend_multisearch_union`: translate.SPL2ToLynxFlow: unsupported command: multisearch (use union in LynxFlow)
- `cross_multisearch_summary`: translate.SPL2ToLynxFlow: unsupported command: multisearch (use union in LynxFlow)
- `multisearch_cross_agg`: translate.SPL2ToLynxFlow: unsupported command: multisearch (use union in LynxFlow)
- `multisearch_cross_index`: translate.SPL2ToLynxFlow: unsupported command: multisearch (use union in LynxFlow)

## unsupported command: streamstats (6)

- `backend_streamstats_by_service`: translate.SPL2ToLynxFlow: unsupported command: streamstats (order-dependent running aggregate)
- `backend_streamstats_cumulative`: translate.SPL2ToLynxFlow: unsupported command: streamstats (order-dependent running aggregate)
- `backend_streamstats_sorted_tail`: translate.SPL2ToLynxFlow: unsupported command: streamstats (order-dependent running aggregate)
- `backend_streamstats_window`: translate.SPL2ToLynxFlow: unsupported command: streamstats (order-dependent running aggregate)
- `backend_streamstats`: translate.SPL2ToLynxFlow: unsupported command: streamstats (order-dependent running aggregate)
- `backend_streamstats_rolling`: translate.SPL2ToLynxFlow: unsupported command: streamstats (order-dependent running aggregate)

## unsupported command: transaction (3)

- `backend_transaction_maxspan`: translate.SPL2ToLynxFlow: unsupported command: transaction (cross-event session state)
- `backend_transaction_user`: translate.SPL2ToLynxFlow: unsupported command: transaction (cross-event session state)
- `backend_transaction_user`: translate.SPL2ToLynxFlow: unsupported command: transaction (cross-event session state)

