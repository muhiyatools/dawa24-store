-- Migration 048 Down: Remove Seeded Data

BEGIN;

DELETE FROM hr.job_offers WHERE location IN ('القاهرة - مدينة نصر', 'الجيزة - الدقي', 'الإسكندرية والبحيرة', 'القاهرة - العبور', 'القاهرة - مصر الجديدة');
DELETE FROM catalog.products WHERE sku IN ('AUG-1G-14T', 'PAN-EXT-24T', 'CON-5MG-30T', 'GLU-1000-30T', 'CAT-50MG-20T', 'VEN-INH-200D', 'LAN-SOLO-5P');
DELETE FROM org.organizations WHERE organization_number IN ('ORG-EGY-1001', 'ORG-EGY-1002', 'ORG-EGY-1003', 'ORG-EGY-1004', 'ORG-PHARM-2001', 'ORG-PHARM-2002', 'ORG-PHARM-2003');

COMMIT;
