package chainview

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenGenerationReader uses the dedicated checkpoint reader login without
// acquiring an ingest lease or creating deployment identities.
func OpenGenerationReader(ctx context.Context, dsn string) (*Repository, error) {
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errors.New("invalid rental checkpoint database configuration")
	}
	var allowed bool
	err = db.QueryRow(ctx, `SELECT current_user=session_user
		AND pg_has_role(current_user,'nu54_p07_rental_reader','USAGE')
		AND NOT EXISTS (SELECT FROM pg_roles WHERE rolname IN (
			'nu54_p07_ingest','nu54_p07_switch','nu54_p07_legacy','nu54_p07_incident',
			'nu54_p07_public','nu54_p07_reconcile','nu54_p07_explorer','nu54_p07_replay_request')
			AND pg_has_role(current_user,oid,'MEMBER'))
		AND NOT has_schema_privilege('chainview','CREATE')
		AND NOT has_table_privilege('chainview.generation','INSERT,UPDATE,DELETE,TRUNCATE')
		AND NOT has_table_privilege('chainview.deployment_profile','INSERT,UPDATE,DELETE,TRUNCATE')
		AND NOT has_table_privilege('chainview.read_target','INSERT,UPDATE,DELETE,TRUNCATE')`).Scan(&allowed)
	if err != nil || !allowed {
		db.Close()
		return nil, errors.New("rental reconciliation requires the dedicated checkpoint reader login")
	}
	return &Repository{db: db}, nil
}

// CheckGenerationReady confirms the configured profile already has a complete
// live read target. The reconciliation route repeats these checks at request time.
func (r *Reader) CheckGenerationReady(ctx context.Context) error {
	anchor, err := generationAnchor(ctx, r.DB.db, r.Profile)
	source, ok := r.Source.(generationStateSource)
	if err != nil || !ok {
		return errGenerationUnresolved
	}
	if !currentGenerationAnchor(ctx, source, r.Profile, anchor) {
		return errGenerationUnresolved
	}
	return nil
}

// ConfirmGenerationAssessment rechecks the current pinned finalized checkpoint
// after the registry recheck. It publishes no persisted proof.
func (r *Reader) ConfirmGenerationAssessment(ctx context.Context, assessment GenerationAssessment) bool {
	if assessment.AnchorHeight == nil || assessment.Binding.Stream != r.Profile.StreamID ||
		assessment.Binding.StreamGeneration != r.Profile.GenerationID {
		return false
	}
	source, ok := r.Source.(generationStateSource)
	if !ok {
		return false
	}
	return pgx.BeginFunc(ctx, r.DB.db, func(tx pgx.Tx) error {
		var locked bool
		err := tx.QueryRow(ctx, `SELECT chainview.lock_absence_anchor($1,$2,$3,$4)`,
			r.Profile.StreamID, r.Profile.GenerationID, *assessment.AnchorHeight, assessment.AnchorHash).Scan(&locked)
		if err != nil || !locked {
			return errGenerationUnresolved
		}
		anchor, err := generationAnchor(ctx, tx, r.Profile)
		if err != nil || anchor.Height != *assessment.AnchorHeight || anchor.Hash.Hex() != assessment.AnchorHash ||
			anchor.Time.Unix() != assessment.AnchorTime || !currentGenerationAnchor(ctx, source, r.Profile, anchor) {
			return errGenerationUnresolved
		}
		return nil
	}) == nil
}

func currentGenerationAnchor(ctx context.Context, source generationStateSource, profile Profile, anchor Header) bool {
	_, final, err := source.Heads(ctx)
	return err == nil && final.Height >= anchor.Height && verifyAnchor(ctx, source, anchor) == nil &&
		source.generationDeploymentAt(ctx, profile, anchor)
}
