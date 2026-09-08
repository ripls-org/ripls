import 'equivalence_tier.dart';

/// kMoneyPaycheckLadder is the 17-tier "What your community gave one
/// of you" ladder for the community Money Saved metric. Thresholds in
/// USD. Authored verbatim from
/// `docs/cowork/Data Scientist/Ripls_Equivalence_Spec.md`.
const List<EquivalenceTier> kMoneyPaycheckLadder = [
  EquivalenceTier(
    id: 'm_15',
    threshold: 15,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM15',
    copyKey: 'equivalenceMoneyPaycheckCopyM15',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceWhataburgersMenu',
    sourceUrl:
        'https://whataburgersmenu.us/comparing-popular-u-s-restaurant-menu-prices-in-2026/',
  ),
  EquivalenceTier(
    id: 'm_35',
    threshold: 35,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM35',
    copyKey: 'equivalenceMoneyPaycheckCopyM35',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceBlsEarnings',
    sourceUrl: 'https://www.bls.gov/news.release/empsit.t19.htm',
  ),
  EquivalenceTier(
    id: 'm_75',
    threshold: 75,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM75',
    copyKey: 'equivalenceMoneyPaycheckCopyM75',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceUsdaFoodPlans',
    sourceUrl:
        'https://www.fns.usda.gov/research/cnpp/usda-food-plans/cost-food-monthly-reports',
  ),
  EquivalenceTier(
    id: 'm_125',
    threshold: 125,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM125',
    copyKey: 'equivalenceMoneyPaycheckCopyM125',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceWalletHubGas',
    sourceUrl:
        'https://wallethub.com/edu/b/average-gas-cost-per-month/170444',
  ),
  EquivalenceTier(
    id: 'm_200',
    threshold: 200,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM200',
    copyKey: 'equivalenceMoneyPaycheckCopyM200',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceWalletHubGas',
    sourceUrl:
        'https://wallethub.com/edu/b/average-gas-cost-per-month/170444',
  ),
  EquivalenceTier(
    id: 'm_250',
    threshold: 250,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM250',
    copyKey: 'equivalenceMoneyPaycheckCopyM250',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceBlsEarnings',
    sourceUrl: 'https://www.bls.gov/news.release/empsit.t19.htm',
  ),
  EquivalenceTier(
    id: 'm_500',
    threshold: 500,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM500',
    copyKey: 'equivalenceMoneyPaycheckCopyM500',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceRamsey',
    sourceUrl:
        'https://www.ramseysolutions.com/budgeting/average-cost-of-groceries',
  ),
  EquivalenceTier(
    id: 'm_610',
    threshold: 610,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM610',
    copyKey: 'equivalenceMoneyPaycheckCopyM610',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceCrossCountry',
    sourceUrl:
        'https://crosscountrymortgage.com/mortgage/resources/how-much-do-utilities-cost-per-month/',
  ),
  EquivalenceTier(
    id: 'm_1025',
    threshold: 1000,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM1025',
    copyKey: 'equivalenceMoneyPaycheckCopyM1025',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceCensusHousing',
    sourceUrl:
        'https://www.census.gov/library/stories/2026/01/housing-costs.html',
  ),
  EquivalenceTier(
    id: 'm_1500',
    threshold: 1500,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM1500',
    copyKey: 'equivalenceMoneyPaycheckCopyM1500',
    sourceNameKey: 'equivalenceMoneyPaycheckSourcePayscale',
    sourceUrl:
        'https://worldatwork.org/publications/workspan-daily/payscale-u-s-employers-forecast-3-5-pay-increases-for-2026',
  ),
  EquivalenceTier(
    id: 'm_2000',
    threshold: 2000,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM2000',
    copyKey: 'equivalenceMoneyPaycheckCopyM2000',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceCensusHousing',
    sourceUrl:
        'https://www.census.gov/library/stories/2026/01/housing-costs.html',
  ),
  EquivalenceTier(
    id: 'm_3000',
    threshold: 3000,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM3000',
    copyKey: 'equivalenceMoneyPaycheckCopyM3000',
    sourceNameKey: 'equivalenceMoneyPaycheckSourcePayscale',
    sourceUrl:
        'https://worldatwork.org/publications/workspan-daily/payscale-u-s-employers-forecast-3-5-pay-increases-for-2026',
  ),
  EquivalenceTier(
    id: 'm_5000',
    threshold: 5000,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM5000',
    copyKey: 'equivalenceMoneyPaycheckCopyM5000',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceBlsPaidLeave',
    sourceUrl:
        'https://www.bls.gov/charts/employee-benefits/paid-leave-sick-vacation-days-by-service-requirement.htm',
  ),
  EquivalenceTier(
    id: 'm_7500',
    threshold: 7500,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM7500',
    copyKey: 'equivalenceMoneyPaycheckCopyM7500',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceIrsLimits',
    sourceUrl:
        'https://www.irs.gov/newsroom/401k-limit-increases-to-24500-for-2026-ira-limit-increases-to-7500',
  ),
  EquivalenceTier(
    id: 'm_10000',
    threshold: 10000,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM10000',
    copyKey: 'equivalenceMoneyPaycheckCopyM10000',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceCensusHousing',
    sourceUrl:
        'https://www.census.gov/library/stories/2026/01/housing-costs.html',
  ),
  EquivalenceTier(
    id: 'm_24500',
    threshold: 24500,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM24500',
    copyKey: 'equivalenceMoneyPaycheckCopyM24500',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceIrsLimits',
    sourceUrl:
        'https://www.irs.gov/newsroom/401k-limit-increases-to-24500-for-2026-ira-limit-increases-to-7500',
  ),
  EquivalenceTier(
    id: 'm_50000',
    threshold: 50000,
    unit: EquivalenceUnit.usd,
    labelKey: 'equivalenceMoneyPaycheckLabelM50000',
    copyKey: 'equivalenceMoneyPaycheckCopyM50000',
    sourceNameKey: 'equivalenceMoneyPaycheckSourceBlsEarnings',
    sourceUrl: 'https://www.bls.gov/news.release/empsit.t19.htm',
  ),
];
