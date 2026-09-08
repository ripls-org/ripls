import 'equivalence_tier.dart';

/// kCo2DailyLifeLadder is the 17-tier "What your community kept out of
/// the air" ladder for the community CO₂ Avoided metric. Thresholds in
/// kilograms of CO₂. Authored verbatim from
/// `docs/cowork/Data Scientist/Ripls_Equivalence_Spec.md` v1.1.
const List<EquivalenceTier> kCo2DailyLifeLadder = [
  EquivalenceTier(
    id: 'c_05',
    threshold: 0.5,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC05',
    copyKey: 'equivalenceCo2DailyLifeCopyC05',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceEpaVehicle',
    sourceUrl:
        'https://www.epa.gov/greenvehicles/greenhouse-gas-emissions-typical-passenger-vehicle',
  ),
  EquivalenceTier(
    id: 'c_1',
    threshold: 1,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC1',
    copyKey: 'equivalenceCo2DailyLifeCopyC1',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceEpaVehicle',
    sourceUrl:
        'https://www.epa.gov/greenvehicles/greenhouse-gas-emissions-typical-passenger-vehicle',
  ),
  EquivalenceTier(
    id: 'c_2',
    threshold: 2,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC2',
    copyKey: 'equivalenceCo2DailyLifeCopyC2',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceCo2EverythingLaundry',
    sourceUrl:
        'https://www.co2everything.com/co2e-of/washing-load-40c-with-dryer',
  ),
  EquivalenceTier(
    id: 'c_35',
    threshold: 3.5,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC35',
    copyKey: 'equivalenceCo2DailyLifeCopyC35',
    sourceNameKey: 'equivalenceCo2DailyLifeSourcePlantBasedMinutes',
    sourceUrl:
        'https://plantbasedminutes.com/article/the-carbon-cost-of-one-beef-burger',
  ),
  EquivalenceTier(
    id: 'c_5',
    threshold: 5,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC5',
    copyKey: 'equivalenceCo2DailyLifeCopyC5',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceCo2EverythingLaundry',
    sourceUrl:
        'https://www.co2everything.com/co2e-of/washing-load-40c-with-dryer',
  ),
  EquivalenceTier(
    id: 'c_9',
    threshold: 9,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC9',
    copyKey: 'equivalenceCo2DailyLifeCopyC9',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceEpaEquivalencies',
    sourceUrl:
        'https://www.epa.gov/energy/greenhouse-gas-equivalencies-calculator-calculations-and-references',
  ),
  EquivalenceTier(
    id: 'c_15',
    threshold: 15,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC15',
    copyKey: 'equivalenceCo2DailyLifeCopyC15',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceEpaVehicle',
    sourceUrl:
        'https://www.epa.gov/greenvehicles/greenhouse-gas-emissions-typical-passenger-vehicle',
  ),
  EquivalenceTier(
    id: 'c_25',
    threshold: 25,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC25',
    copyKey: 'equivalenceCo2DailyLifeCopyC25',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceEiaKwh',
    sourceUrl: 'https://www.eia.gov/tools/faqs/faq.php?id=74&t=11',
  ),
  EquivalenceTier(
    id: 'c_50',
    threshold: 50,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC50',
    copyKey: 'equivalenceCo2DailyLifeCopyC50',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceOwidTravel',
    sourceUrl: 'https://ourworldindata.org/travel-carbon-footprint',
  ),
  EquivalenceTier(
    id: 'c_100',
    threshold: 100,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC100',
    copyKey: 'equivalenceCo2DailyLifeCopyC100',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceEpaEquivalencies',
    sourceUrl:
        'https://www.epa.gov/energy/greenhouse-gas-equivalencies-calculator-calculations-and-references',
  ),
  EquivalenceTier(
    id: 'c_250',
    threshold: 250,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC250',
    copyKey: 'equivalenceCo2DailyLifeCopyC250',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceEpaVehicle',
    sourceUrl:
        'https://www.epa.gov/greenvehicles/greenhouse-gas-emissions-typical-passenger-vehicle',
  ),
  EquivalenceTier(
    id: 'c_500',
    threshold: 500,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC500',
    copyKey: 'equivalenceCo2DailyLifeCopyC500',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceOwidTravel',
    sourceUrl: 'https://ourworldindata.org/travel-carbon-footprint',
  ),
  EquivalenceTier(
    id: 'c_1000',
    threshold: 1000,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC1000',
    copyKey: 'equivalenceCo2DailyLifeCopyC1000',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceOwidAviation',
    sourceUrl: 'https://ourworldindata.org/breakdown-co2-aviation',
  ),
  EquivalenceTier(
    id: 'c_2500',
    threshold: 2500,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC2500',
    copyKey: 'equivalenceCo2DailyLifeCopyC2500',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceEpaVehicle',
    sourceUrl:
        'https://www.epa.gov/greenvehicles/greenhouse-gas-emissions-typical-passenger-vehicle',
  ),
  EquivalenceTier(
    id: 'c_5000',
    threshold: 5000,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC5000',
    copyKey: 'equivalenceCo2DailyLifeCopyC5000',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceEpaHousehold',
    sourceUrl:
        'https://www.epa.gov/ghgemissions/assumptions-and-references-household-carbon-footprint-calculator',
  ),
  EquivalenceTier(
    id: 'c_15000',
    threshold: 15000,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC15000',
    copyKey: 'equivalenceCo2DailyLifeCopyC15000',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceOwidUsProfile',
    sourceUrl: 'https://ourworldindata.org/profile/co2/united-states',
  ),
  EquivalenceTier(
    id: 'c_50000',
    threshold: 50000,
    unit: EquivalenceUnit.kg,
    labelKey: 'equivalenceCo2DailyLifeLabelC50000',
    copyKey: 'equivalenceCo2DailyLifeCopyC50000',
    sourceNameKey: 'equivalenceCo2DailyLifeSourceOwidUsProfile',
    sourceUrl: 'https://ourworldindata.org/profile/co2/united-states',
  ),
];
