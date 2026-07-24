-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE FUNCTION managed_update_fixture()
RETURNS void
LANGUAGE plpgsql
AS $function$
BEGIN
    PERFORM 'semicolons; and DROP TABLE text stay in the function body';
END;
$function$;
