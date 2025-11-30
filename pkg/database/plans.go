package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/sirupsen/logrus"
)

// CreatePlan creates a new plan
func (d *DiskDB) CreatePlan(plan *models.Plan) error {
	if err := plan.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := plan.MarshalForDB(); err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	now := time.Now().Unix()
	plan.CreatedAt = now
	plan.UpdatedAt = now

	log.WithFields(logrus.Fields{
		"name": plan.Name,
		"mode": plan.Mode,
	}).Info("Creating plan")

	query := `
		INSERT INTO plans (name, description, mode, status, sources_json, conditions_json, outcomes_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := d.db.Exec(query,
		plan.Name,
		plan.Description,
		plan.Mode,
		plan.Status,
		plan.SourcesJSON,
		plan.ConditionsJSON,
		plan.OutcomesJSON,
		plan.CreatedAt,
		plan.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert plan: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get plan ID: %w", err)
	}
	plan.ID = id

	return nil
}

// GetPlan retrieves a plan by name
func (d *DiskDB) GetPlan(name string) (*models.Plan, error) {
	query := `SELECT id, name, description, mode, status, sources_json, conditions_json, outcomes_json, created_at, updated_at, last_run_at
			  FROM plans WHERE name = ?`

	var plan models.Plan
	var description sql.NullString
	var conditionsJSON sql.NullString
	var lastRunAt sql.NullInt64

	err := d.db.QueryRow(query, name).Scan(
		&plan.ID,
		&plan.Name,
		&description,
		&plan.Mode,
		&plan.Status,
		&plan.SourcesJSON,
		&conditionsJSON,
		&plan.OutcomesJSON,
		&plan.CreatedAt,
		&plan.UpdatedAt,
		&lastRunAt,
	)
	if IsNotFound(err) {
		return nil, fmt.Errorf("plan not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	if description.Valid {
		plan.Description = &description.String
	}
	if conditionsJSON.Valid {
		condStr := conditionsJSON.String
		plan.ConditionsJSON = &condStr
	}
	if lastRunAt.Valid {
		plan.LastRunAt = &lastRunAt.Int64
	}

	if err := plan.UnmarshalFromDB(); err != nil {
		return nil, err
	}

	return &plan, nil
}

// ListPlans returns all plans
func (d *DiskDB) ListPlans() ([]*models.Plan, error) {
	query := `SELECT id, name, description, mode, status, sources_json, conditions_json, outcomes_json, created_at, updated_at, last_run_at
			  FROM plans ORDER BY created_at DESC`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list plans: %w", err)
	}
	defer rows.Close()

	var plans []*models.Plan
	for rows.Next() {
		var plan models.Plan
		var description sql.NullString
		var conditionsJSON sql.NullString
		var lastRunAt sql.NullInt64

		if err := rows.Scan(
			&plan.ID,
			&plan.Name,
			&description,
			&plan.Mode,
			&plan.Status,
			&plan.SourcesJSON,
			&conditionsJSON,
			&plan.OutcomesJSON,
			&plan.CreatedAt,
			&plan.UpdatedAt,
			&lastRunAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan plan: %w", err)
		}

		if description.Valid {
			plan.Description = &description.String
		}
		if conditionsJSON.Valid {
			condStr := conditionsJSON.String
			plan.ConditionsJSON = &condStr
		}
		if lastRunAt.Valid {
			plan.LastRunAt = &lastRunAt.Int64
		}

		if err := plan.UnmarshalFromDB(); err != nil {
			return nil, err
		}

		plans = append(plans, &plan)
	}

	return plans, nil
}

// UpdatePlan updates an existing plan
func (d *DiskDB) UpdatePlan(plan *models.Plan) error {
	if err := plan.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := plan.MarshalForDB(); err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	plan.UpdatedAt = time.Now().Unix()

	log.WithFields(logrus.Fields{
		"name": plan.Name,
	}).Info("Updating plan")

	query := `
		UPDATE plans
		SET description = ?, mode = ?, status = ?, sources_json = ?, conditions_json = ?, outcomes_json = ?, updated_at = ?
		WHERE name = ?
	`

	result, err := d.db.Exec(query,
		plan.Description,
		plan.Mode,
		plan.Status,
		plan.SourcesJSON,
		plan.ConditionsJSON,
		plan.OutcomesJSON,
		plan.UpdatedAt,
		plan.Name,
	)
	if err != nil {
		return fmt.Errorf("failed to update plan: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("plan not found: %s", plan.Name)
	}

	return nil
}

// DeletePlan deletes a plan by name
func (d *DiskDB) DeletePlan(name string) error {
	log.WithField("name", name).Info("Deleting plan")

	query := `DELETE FROM plans WHERE name = ?`
	result, err := d.db.Exec(query, name)
	if err != nil {
		return fmt.Errorf("failed to delete plan: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("plan not found: %s", name)
	}

	return nil
}

// CreatePlanExecution creates a new execution record
func (d *DiskDB) CreatePlanExecution(planID int64, planName string) (*models.PlanExecution, error) {
	exec := &models.PlanExecution{
		PlanID:    planID,
		PlanName:  planName,
		StartedAt: time.Now().Unix(),
		Status:    "running",
	}

	query := `INSERT INTO plan_executions (plan_id, plan_name, started_at, status)
			  VALUES (?, ?, ?, ?)`

	result, err := d.db.Exec(query, exec.PlanID, exec.PlanName, exec.StartedAt, exec.Status)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get execution ID: %w", err)
	}
	exec.ID = id

	log.WithFields(logrus.Fields{
		"plan_id":      planID,
		"plan_name":    planName,
		"execution_id": exec.ID,
	}).Info("Created plan execution")

	return exec, nil
}

// UpdatePlanExecution updates execution record with results
func (d *DiskDB) UpdatePlanExecution(exec *models.PlanExecution) error {
	query := `
		UPDATE plan_executions
		SET completed_at = ?, duration_ms = ?, entries_processed = ?, entries_matched = ?, outcomes_applied = ?, status = ?, error_message = ?
		WHERE id = ?
	`

	_, err := d.db.Exec(query,
		exec.CompletedAt,
		exec.DurationMs,
		exec.EntriesProcessed,
		exec.EntriesMatched,
		exec.OutcomesApplied,
		exec.Status,
		exec.ErrorMessage,
		exec.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update execution: %w", err)
	}

	log.WithFields(logrus.Fields{
		"execution_id": exec.ID,
		"status":       exec.Status,
		"matched":      exec.EntriesMatched,
	}).Info("Updated plan execution")

	return nil
}

// UpdatePlanLastRun updates the last_run_at timestamp
func (d *DiskDB) UpdatePlanLastRun(planID int64) error {
	now := time.Now().Unix()
	query := `UPDATE plans SET last_run_at = ? WHERE id = ?`
	_, err := d.db.Exec(query, now, planID)
	return err
}

// GetPlanExecutions retrieves execution history for a plan
func (d *DiskDB) GetPlanExecutions(planName string, limit int) ([]*models.PlanExecution, error) {
	query := `SELECT id, plan_id, plan_name, started_at, completed_at, duration_ms, entries_processed, entries_matched, outcomes_applied, status, error_message
			  FROM plan_executions WHERE plan_name = ? ORDER BY started_at DESC LIMIT ?`

	rows, err := d.db.Query(query, planName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get executions: %w", err)
	}
	defer rows.Close()

	var executions []*models.PlanExecution
	for rows.Next() {
		var exec models.PlanExecution
		var completedAt sql.NullInt64
		var durationMs sql.NullInt64
		var errorMessage sql.NullString

		if err := rows.Scan(
			&exec.ID,
			&exec.PlanID,
			&exec.PlanName,
			&exec.StartedAt,
			&completedAt,
			&durationMs,
			&exec.EntriesProcessed,
			&exec.EntriesMatched,
			&exec.OutcomesApplied,
			&exec.Status,
			&errorMessage,
		); err != nil {
			return nil, fmt.Errorf("failed to scan execution: %w", err)
		}

		if completedAt.Valid {
			exec.CompletedAt = &completedAt.Int64
		}
		if durationMs.Valid {
			dms := int(durationMs.Int64)
			exec.DurationMs = &dms
		}
		if errorMessage.Valid {
			exec.ErrorMessage = &errorMessage.String
		}

		executions = append(executions, &exec)
	}

	return executions, nil
}

// RecordPlanOutcome creates an outcome record
func (d *DiskDB) RecordPlanOutcome(record *models.PlanOutcomeRecord) error {
	record.CreatedAt = time.Now().Unix()

	query := `INSERT INTO plan_outcome_records (execution_id, plan_id, entry_path, outcome_type, outcome_data, status, error_message, created_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := d.db.Exec(query,
		record.ExecutionID,
		record.PlanID,
		record.EntryPath,
		record.OutcomeType,
		record.OutcomeData,
		record.Status,
		record.ErrorMessage,
		record.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record outcome: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get outcome ID: %w", err)
	}
	record.ID = id

	return nil
}
