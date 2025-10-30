import React from "react";
import PropTypes from "prop-types";
import { Box, Paper } from "@mui/material";
import { DndProvider, useDrag, useDrop } from "react-dnd";
import { HTML5Backend } from "react-dnd-html5-backend";
import { isDark } from "../utils/isDark";
import SectionHeader from "./SectionHeader";

const ItemTypes = { CHIP: "chip" };

function TmdbChip({ tmdbType, plexType, onMove }) {
  const [{ isDragging }, drag] = useDrag({
    type: ItemTypes.CHIP,
    item: { tmdbType, plexType },
    collect: (monitor) => ({ isDragging: monitor.isDragging() }),
  });
  return (
    <Box
      ref={drag}
      sx={{
        background: isDark ? "#222" : "#e5e7eb",
        color: isDark ? "#e5e7eb" : "#222",
        borderRadius: 2,
        fontSize: 13,
        height: 24,
        lineHeight: "24px",
        margin: "2px 6px 2px 0",
        padding: "0 10px",
        fontWeight: 500,
        display: "flex",
        alignItems: "center",
        opacity: isDragging ? 0.5 : 1,
        cursor: "grab",
      }}
      title="Drag to another Plex type"
    >
      {tmdbType}
      <Box
        component="span"
        sx={{
          cursor: "pointer",
          marginLeft: 6,
          color: isDark ? "#c084fc" : "#a855f7",
          fontWeight: 700,
          fontSize: 15,
        }}
        onClick={(e) => {
          e.stopPropagation();
          onMove(tmdbType, "Other");
        }}
        title="Remove assignment"
      >
        ×
      </Box>
    </Box>
  );
}

ExtrasTypeMappingConfig.propTypes = {
  mapping: PropTypes.object,
  onMappingChange: PropTypes.func,
  tmdbTypes: PropTypes.array,
  plexTypes: PropTypes.array,
};

ExtrasTypeMappingConfig.defaultProps = {
  mapping: {},
  onMappingChange: null,
  tmdbTypes: [],
  plexTypes: [],
};

function PlexTypeBox({ plexType, onDropChip, children }) {
  const [{ isOver, canDrop }, drop] = useDrop({
    accept: ItemTypes.CHIP,
    drop: (item) => onDropChip(item.tmdbType, plexType),
    canDrop: (item) => item.plexType !== plexType,
    collect: (monitor) => ({
      isOver: monitor.isOver(),
      canDrop: monitor.canDrop(),
    }),
  });

  const activeBackground = isDark ? "#047857" : "#d1fae5";
  const idleBackground = isDark ? "#333" : "#f5f5f5";
  const backgroundColor = isOver && canDrop ? activeBackground : idleBackground;

  return (
    <Box
      ref={drop}
      sx={{
        display: "flex",
        flexWrap: "wrap",
        alignItems: "center",
        minHeight: 36,
        background: backgroundColor,
        border: isDark ? "1px solid #e5e7eb" : "1px solid #000",
        borderRadius: 2,
        padding: "4px 8px",
        transition: "background 0.2s",
      }}
    >
      {children}
    </Box>
  );
}

PlexTypeBox.propTypes = {
  plexType: PropTypes.oneOfType([PropTypes.string, PropTypes.number])
    .isRequired,
  onDropChip: PropTypes.func,
  children: PropTypes.node,
};

PlexTypeBox.defaultProps = {
  onDropChip: () => {},
  children: null,
};

export default function ExtrasTypeMappingConfig({
  mapping,
  onMappingChange,
  tmdbTypes,
  plexTypes,
}) {
  const handleMoveChip = (tmdbType, newPlexType) => {
    if (onMappingChange) {
      onMappingChange({ ...mapping, [tmdbType]: newPlexType });
    }
  };

  return (
    <DndProvider backend={HTML5Backend}>
      <Box>
        <SectionHeader>TMDB to Plex Extra Type Mapping</SectionHeader>
        <Paper
          sx={{
            mt: 2,
            p: 1,
            maxWidth: 470,
            ml: 0,
            boxShadow: "none",
            border: "none",
            background: "transparent",
            color: isDark ? "#e5e7eb" : "#222",
          }}
        >
          {(plexTypes || []).map((plexTypeObj) => {
            // plexTypeObj expected shape: { key, label, value }
            const plexKey = plexTypeObj?.key ?? String(plexTypeObj);
            const plexLabel = plexTypeObj?.label ?? String(plexTypeObj);
            // mapping values are the display label (e.g. "Trailers"), so compare to plexLabel
            const assignedTmdbTypes = (tmdbTypes || []).filter(
              (tmdbType) => mapping[tmdbType] === plexLabel,
            );
            return (
              <Box key={plexKey} display="flex" alignItems="center" mb={1}>
                <Box
                  minWidth={140}
                  fontWeight={500}
                  fontSize={14}
                  textAlign="left"
                  sx={{ color: isDark ? "#e5e7eb" : "#222" }}
                >
                  {plexLabel}
                </Box>
                <Box flex={1} ml={1}>
                  {/* Pass the display label to the drop handler so mapping stores the label value */}
                  <PlexTypeBox
                    plexType={plexKey}
                    assignedTmdbTypes={assignedTmdbTypes}
                    onDropChip={(tmdbType) =>
                      handleMoveChip(tmdbType, plexLabel)
                    }
                  >
                    {assignedTmdbTypes.map((tmdbType) => (
                      <TmdbChip
                        key={tmdbType}
                        tmdbType={tmdbType}
                        plexType={plexLabel}
                        onMove={handleMoveChip}
                      />
                    ))}
                  </PlexTypeBox>
                </Box>
              </Box>
            );
          })}
        </Paper>
      </Box>
    </DndProvider>
  );
}

TmdbChip.propTypes = {
  tmdbType: PropTypes.string.isRequired,
  plexType: PropTypes.string,
  onMove: PropTypes.func.isRequired,
};

TmdbChip.defaultProps = {
  plexType: "",
};
