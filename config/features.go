package config

// FeatureSet is a bitfield of enabled features for a market.
type FeatureSet uint32

const (
	FeatureMarketOrders  FeatureSet = 1 << 0
	FeatureIOC           FeatureSet = 1 << 1
	FeatureFOK           FeatureSet = 1 << 2
	FeatureStopOrders    FeatureSet = 1 << 3
	FeatureIcebergOrders FeatureSet = 1 << 4
	FeaturePostOnly      FeatureSet = 1 << 5
	FeatureReduceOnly    FeatureSet = 1 << 6
	FeatureAuctions      FeatureSet = 1 << 7
)

// DefaultFeatures returns the default feature set: market orders + IOC.
func DefaultFeatures() FeatureSet {
	return FeatureMarketOrders | FeatureIOC
}

// Has returns true if all bits in f are set.
func (fs FeatureSet) Has(f FeatureSet) bool { return fs&f == f }

// Add returns fs with all bits in f added.
func (fs FeatureSet) Add(f FeatureSet) FeatureSet { return fs | f }

// Remove returns fs with all bits in f cleared.
func (fs FeatureSet) Remove(f FeatureSet) FeatureSet { return fs &^ f }
